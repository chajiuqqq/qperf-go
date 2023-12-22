package client

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"qperf-go/common"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/logging"
)

const (
	defaultFileName = "result/qperf_result.json"
)

type Client struct {
	state          common.State
	printRaw       bool
	reportInterval time.Duration
	logger         common.Logger
	StatesHistory  []*States
}

type States struct {
	RateBytes float64
	Bytes     uint64
	Second    int
	Packets   uint64
}

// Run client.
// if proxyAddr is nil, no proxy is used.
func Run(addr net.UDPAddr, timeToFirstByteOnly bool, printRaw bool, createQLog bool, migrateAfter time.Duration, proxyAddr *net.UDPAddr, probeTime time.Duration, reportInterval time.Duration, tlsServerCertFile string, tlsProxyCertFile string, initialCongestionWindow uint32, initialReceiveWindow uint64, maxReceiveWindow uint64, use0RTT bool, useProxy0RTT, allowEarlyHandover bool, useXse bool, logPrefix string, qlogPrefix string) {
	c := Client{
		state:          common.State{},
		printRaw:       printRaw,
		reportInterval: reportInterval,
		StatesHistory:  make([]*States, 0),
	}

	c.logger = common.DefaultLogger.WithPrefix(logPrefix)

	tracers := make([]logging.Tracer, 0)

	tracers = append(tracers, common.StateTracer{
		State: &c.state,
	})

	if createQLog {
		tracers = append(tracers, common.NewQlogTracer(qlogPrefix, c.logger))
	}

	tracers = append(tracers, common.NewEventTracer(common.Handlers{
		UpdatePath: func(odcid logging.ConnectionID, newRemote net.Addr) {
			c.logger.Infof("migrated QUIC connection %s to %s at %.3f s", odcid.String(), newRemote, time.Now().Sub(c.state.GetStartTime()).Seconds())
		},
		StartedConnection: func(odcid logging.ConnectionID, local, remote net.Addr, srcConnID, destConnID logging.ConnectionID) {
			c.logger.Infof("started QUIC connection %s", odcid.String())
		},
		ClosedConnection: func(odcid logging.ConnectionID, err error) {
			c.logger.Infof("closed QUIC connection %s", odcid.String())
		},
	}))

	if initialReceiveWindow > maxReceiveWindow {
		maxReceiveWindow = initialReceiveWindow
	}

	var proxyConf *quic.ProxyConfig

	if proxyAddr != nil {
		proxyConf = &quic.ProxyConfig{
			Addr: proxyAddr.String(),
			TlsConf: &tls.Config{
				RootCAs:            common.NewCertPoolWithCert(tlsProxyCertFile),
				NextProtos:         []string{quic.HQUICProxyALPN},
				ClientSessionCache: tls.NewLRUClientSessionCache(1),
			},
			Config: &quic.Config{
				LoggerPrefix:          "proxy control",
				TokenStore:            quic.NewLRUTokenStore(1, 1),
				EnableActiveMigration: true,
			},
		}
	}

	if useProxy0RTT {
		err := common.PingToGatherSessionTicketAndToken(proxyConf.Addr, proxyConf.TlsConf, proxyConf.Config)
		if err != nil {
			panic(fmt.Errorf("failed to prepare 0-RTT to proxy: %w", err))
		}
		c.logger.Infof("stored session ticket and address token of proxy for 0-RTT")
	}

	var clientSessionCache tls.ClientSessionCache
	if use0RTT {
		clientSessionCache = tls.NewLRUClientSessionCache(1)
	}

	var tokenStore quic.TokenStore
	if use0RTT {
		tokenStore = quic.NewLRUTokenStore(1, 1)
	}

	tlsConf := &tls.Config{
		RootCAs:            common.NewCertPoolWithCert(tlsServerCertFile),
		NextProtos:         []string{common.QperfALPN},
		ClientSessionCache: clientSessionCache,
	}

	conf := quic.Config{
		Tracer: logging.NewMultiplexedTracer(tracers...),
		IgnoreReceived1RTTPacketsUntilFirstPathMigration: proxyAddr != nil, // TODO maybe not necessary for client
		EnableActiveMigration:                            true,
		ProxyConf:                                        proxyConf,
		InitialCongestionWindow:                          initialCongestionWindow,
		InitialStreamReceiveWindow:                       initialReceiveWindow,
		MaxStreamReceiveWindow:                           maxReceiveWindow,
		InitialConnectionReceiveWindow:                   uint64(float64(initialReceiveWindow) * quic.ConnectionFlowControlMultiplier),
		MaxConnectionReceiveWindow:                       uint64(float64(maxReceiveWindow) * quic.ConnectionFlowControlMultiplier),
		TokenStore:                                       tokenStore,
		AllowEarlyHandover:                               allowEarlyHandover,
	}

	if useXse {
		conf.ExtraStreamEncryption = quic.EnforceExtraStreamEncryption
	} else {
		conf.ExtraStreamEncryption = quic.DisableExtraStreamEncryption
	}

	if use0RTT {
		err := common.PingToGatherSessionTicketAndToken(addr.String(), tlsConf, &conf)
		if err != nil {
			panic(fmt.Errorf("failed to prepare 0-RTT: %w", err))
		}
		c.logger.Infof("stored session ticket and token")
	}

	c.state.SetStartTime()

	var connection quic.Connection
	if use0RTT {
		var err error
		connection, err = quic.DialAddrEarly(addr.String(), tlsConf, &conf)
		if err != nil {
			panic(fmt.Errorf("failed to establish connection: %w", err))
		}
	} else {
		var err error
		connection, err = quic.DialAddr(addr.String(), tlsConf, &conf)
		if err != nil {
			panic(fmt.Errorf("failed to establish connection: %w", err))
		}
	}

	c.state.SetEstablishmentTime()
	c.reportEstablishmentTime(&c.state)

	if connection.ExtraStreamEncrypted() {
		c.logger.Infof("use XSE-QUIC")
	}

	// migrate
	if migrateAfter.Nanoseconds() != 0 {
		go func() {
			time.Sleep(migrateAfter)
			addr, err := connection.MigrateUDPSocket()
			if err != nil {
				panic(fmt.Errorf("failed to migrate UDP socket: %w", err))
			}
			c.logger.Infof("migrated to %s", addr.String())
		}()
	}

	// close gracefully on interrupt (CTRL+C)
	intChan := make(chan os.Signal, 1)
	signal.Notify(intChan, os.Interrupt)
	go func() {
		<-intChan
		_ = connection.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "client_closed")
		os.Exit(0)
	}()

	stream, err := connection.OpenStream()
	if err != nil {
		panic(fmt.Errorf("failed to open stream: %w", err))
	}

	// send some date to open stream
	_, err = stream.Write([]byte(common.QPerfStartSendingRequest))
	if err != nil {
		panic(fmt.Errorf("failed to write to stream: %w", err))
	}
	err = stream.Close()
	if err != nil {
		panic(fmt.Errorf("failed to close stream: %w", err))
	}

	err = c.receiveFirstByte(stream)
	if err != nil {
		panic(fmt.Errorf("failed to receive first byte: %w", err))
	}

	c.reportFirstByte(&c.state)

	if !timeToFirstByteOnly {
		go c.receive(stream)

		for {
			if time.Now().Sub(c.state.GetFirstByteTime()) > probeTime {
				break
			}
			time.Sleep(reportInterval)
			c.report(&c.state)
		}
	}

	err = connection.CloseWithError(common.RuntimeReachedErrorCode, "runtime_reached")
	if err != nil {
		panic(fmt.Errorf("failed to close connection: %w", err))
	}

	c.reportTotal(&c.state)
}

func (c *Client) reportEstablishmentTime(state *common.State) {
	establishmentTime := state.EstablishmentTime().Sub(state.StartTime())
	if c.printRaw {
		c.logger.Infof("connection establishment time: %f s",
			establishmentTime.Seconds())
	} else {
		c.logger.Infof("connection establishment time: %s",
			humanize.SIWithDigits(establishmentTime.Seconds(), 2, "s"))
	}
}

func (c *Client) reportFirstByte(state *common.State) {
	if c.printRaw {
		c.logger.Infof("time to first byte: %f s",
			state.GetFirstByteTime().Sub(state.StartTime()).Seconds())
	} else {
		c.logger.Infof("time to first byte: %s",
			humanize.SIWithDigits(state.GetFirstByteTime().Sub(state.StartTime()).Seconds(), 2, "s"))
	}
}

func (c *Client) report(state *common.State) {
	receivedBytes, receivedPackets, delta := state.GetAndResetReport()

	if c.printRaw {
		c.logger.Infof("second %f: %f bit/s, bytes received: %d B, packets received: %d",
			time.Now().Sub(state.GetFirstByteTime()).Seconds(),
			float64(receivedBytes)*8/delta.Seconds(),
			receivedBytes,
			receivedPackets)
	} else if c.reportInterval == time.Second {
		c.logger.Infof("second %.0f: %s, bytes received: %s, packets received: %d",
			time.Now().Sub(state.GetFirstByteTime()).Seconds(),
			humanize.SIWithDigits(float64(receivedBytes)*8/delta.Seconds(), 2, "bit/s"),
			humanize.SI(float64(receivedBytes), "B"),
			receivedPackets)
	} else {
		c.logger.Infof("second %.1f: %s, bytes received: %s, packets received: %d",
			time.Now().Sub(state.GetFirstByteTime()).Seconds(),
			humanize.SIWithDigits(float64(receivedBytes)*8/delta.Seconds(), 2, "bit/s"),
			humanize.SI(float64(receivedBytes), "B"),
			receivedPackets)
	}
	c.StatesHistory = append(c.StatesHistory, &States{
		RateBytes: float64(receivedBytes) / delta.Seconds(),
		Bytes:     receivedBytes,
		Second:    int(time.Now().Sub(state.GetFirstByteTime()).Seconds()),
		Packets:   receivedPackets,
	})
}

func (c *Client) reportTotal(state *common.State) {
	receivedBytes, receivedPackets := state.Total()
	if c.printRaw {
		c.logger.Infof("total: bytes received: %d B, packets received: %d",
			receivedBytes,
			receivedPackets)
	} else {
		c.logger.Infof("total: bytes received: %s, packets received: %d",
			humanize.SI(float64(receivedBytes), "B"),
			receivedPackets)
	}
	if err := c.exportStates(defaultFileName); err == nil {
		c.logger.Infof("export states success:%s", defaultFileName)
	} else {
		c.logger.Infof("export states error:%s", err.Error())
	}

}

func (c *Client) receiveFirstByte(stream quic.ReceiveStream) error {
	buf := make([]byte, 1)
	for {
		received, err := stream.Read(buf)
		if err != nil {
			return err
		}
		if received != 0 {
			c.state.AddReceivedBytes(uint64(received))
			return nil
		}
	}
}

func (c *Client) receive(reader io.Reader) {
	buf := make([]byte, 65536)
	for {
		received, err := reader.Read(buf)
		c.state.AddReceivedBytes(uint64(received))
		if err != nil {
			switch err := err.(type) {
			case *quic.ApplicationError:
				if err.ErrorCode == common.RuntimeReachedErrorCode {
					return
				}
			default:
				panic(err)
			}
		}
	}
}

// '[{"col 1":"a","col 2":"b"},{"col 1":"c","col 2":"d"}]' 形式导出
// pd.read_json(_, orient='records') 导入
func (c *Client) exportStates(fileName string) error {
	b, err := json.MarshalIndent(c.StatesHistory, "", "\t")
	if err != nil {
		return err
	}
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(b)
	if err != nil {
		return err
	}
	return nil
}
