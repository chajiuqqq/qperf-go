package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/apernet/quic-go"
	"github.com/apernet/quic-go/qlog"
	"html/template"
	"net"
	"net/http"
	"os"
	"qperf-go/common"
	"qperf-go/internal/congestion"
	"qperf-go/internal/congestion/rl"
	"strings"
	"time"

	"github.com/apernet/quic-go/http3"
	"github.com/gin-gonic/gin"
)

// Run server.
// if proxyAddr is nil, no proxy is used.
func Run(addr net.UDPAddr, createQLog bool, migrateAfter time.Duration, tlsServerCertFile string, tlsServerKeyFile string, initialCongestionWindow uint32, minCongestionWindow uint32, maxCongestionWindow uint32, initialReceiveWindow uint64, maxReceiveWindow uint64, noXse bool, logPrefix string, qlogPrefix string, http3enabled bool, www string, redisAddr string, cc string) {

	logger := common.DefaultLogger.WithPrefix(logPrefix)

	// tracers := make([]logging.Tracer, 0)

	tracer := qlog.DefaultTracer
	if !createQLog {
		// tracers = append(tracers, common.NewQlogTracer(qlogPrefix, c.logger))
		tracer = nil
	}

	// TODO somehow associate it with the qperf session for logging

	// tracers = append(tracers, common.NewEventTracer(common.Handlers{
	// 	UpdatePath: func(odcid logging.ConnectionID, newRemote net.Addr) {
	// 		logger.Infof("migrated QUIC connection %s to %s", odcid.String(), newRemote)
	// 	},
	// 	StartedConnection: func(odcid logging.ConnectionID, local, remote net.Addr, srcConnID, destConnID logging.ConnectionID) {
	// 		logger.Infof("started QUIC connection %s", odcid.String())
	// 	},
	// 	ClosedConnection: func(odcid logging.ConnectionID, err error) {
	// 		logger.Infof("closed QUIC connection %s", odcid.String())
	// 	},
	// }))

	if initialReceiveWindow > maxReceiveWindow {
		maxReceiveWindow = initialReceiveWindow
	}

	if initialCongestionWindow < minCongestionWindow {
		initialCongestionWindow = minCongestionWindow
	}

	conf := quic.Config{
		Tracer: tracer,
		// EnableActiveMigration:          true,
		// InitialCongestionWindow:        initialCongestionWindow,
		// MinCongestionWindow:            minCongestionWindow,
		// MaxCongestionWindow:            maxCongestionWindow,
		InitialStreamReceiveWindow: initialReceiveWindow,
		MaxStreamReceiveWindow:     maxReceiveWindow,
		// InitialConnectionReceiveWindow: uint64(float64(initialReceiveWindow) * quic.ConnectionFlowControlMultiplier),
		// MaxConnectionReceiveWindow:     uint64(float64(maxReceiveWindow) * quic.ConnectionFlowControlMultiplier),
		// TODO add option to disable mtu discovery
		// TODO add option to enable address prevalidation
	}

	// if noXse {
	// 	conf.ExtraStreamEncryption = quic.DisableExtraStreamEncryption
	// } else {
	// 	conf.ExtraStreamEncryption = quic.PreferExtraStreamEncryption
	// }

	// TODO make CLI option
	tlsCert, err := tls.LoadX509KeyPair(tlsServerCertFile, tlsServerKeyFile)
	if err != nil {
		panic(err)
	}

	tlsConf := tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"qperf"},
	}

	// http3
	if http3enabled {
		logger.Infof("http3 served on %s", addr.String())
		handler := setupHandler(www)
		server := http3.Server{
			Handler:    handler,
			Addr:       addr.String(),
			QuicConfig: &conf,
			TLSConfig:  &tlsConf,
		}
		err = server.ListenAndServe()
		if err != nil {
			panic(err)
		}
		return
	}

	listener, err := quic.ListenAddrEarly(addr.String(), &tlsConf, &conf)
	if err != nil {
		panic(err)
	}

	// print new reno as this is the only option in quic-go
	logger.Infof("starting server with pid %d, port %d, cc new reno", os.Getpid(), addr.Port)

	// migrate
	// if migrateAfter.Nanoseconds() != 0 {
	// 	go func() {
	// 		time.Sleep(migrateAfter)
	// 		addr, err := listener.MigrateUDPSocket()
	// 		if err != nil {
	// 			panic(err)
	// 		}
	// 		logger.Infof("migrated to %s", addr.String())
	// 	}()
	// }

	var nextConnectionId uint64 = 0
	redisAddrSplits := strings.Split(redisAddr, ":")
	redisConf := rl.RedisConf{
		Host: redisAddrSplits[0],
		Port: redisAddrSplits[1],
	}
	for {
		quicConnection, err := listener.Accept(context.Background())
		if err != nil {
			panic(err)
		}

		// cc
		switch cc {
		case common.CC_CUBIC:
		case common.CC_RL:
			congestion.UseRL(quicConnection, &redisConf)
		case common.CC_BRUTAL:
			congestion.UseBrutal(quicConnection, uint64(5*1024*1024))
		default:
			panic("invalid cc:" + cc)
		}
		logger.Infof("using %s cc", cc)

		qperfSession := &qperfServerSession{
			connection:   quicConnection,
			connectionID: nextConnectionId,
			logger:       logger.WithPrefix(fmt.Sprintf("connection %d", nextConnectionId)),
		}

		go qperfSession.run()
		nextConnectionId += 1
	}
}

// See https://en.wikipedia.org/wiki/Lehmer_random_number_generator
func generatePRData(l int) []byte {
	res := make([]byte, l)
	seed := uint64(1)
	for i := 0; i < l; i++ {
		seed = seed * 48271 % 2147483647
		res[i] = byte(seed)
	}
	return res
}

var html = template.Must(template.New("https").Parse(`
<html>
<head>
  <title>Https Test</title>
</head>
<body>
  <h1 style="color:red;">Img:{{ .filename }}</h1>
  <img src="www/{{ .filename }}"></img>
</body>
</html>
`))

func setupHandler(www string) http.Handler {

	r := gin.Default()
	r.SetHTMLTemplate(html)
	r.Static("/www", www)

	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "success",
		})
	})
	r.GET("/html/:filename", func(c *gin.Context) {
		c.HTML(http.StatusOK, "https", gin.H{
			"status":   "success",
			"filename": c.Param("filename"),
		})
	})
	return r
}
