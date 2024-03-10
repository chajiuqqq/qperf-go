package rl

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/apernet/quic-go/congestion"
	"os"
	"qperf-go/internal/congestion/common"
	"strconv"
	"time"
)

const (
	debugEnv                = "HYSTERIA_BRUTAL_DEBUG"
	debugPrintInterval      = 2
	initialCongestionWindow = 20
	MQ_CONNECTION_ID        = "test1234"
)

var _ congestion.CongestionControl = &RLSender{}

type RedisConf struct {
	Host string
	Port string
}
type RLSender struct {
	rttStats        congestion.RTTStatsProvider
	maxDatagramSize congestion.ByteCount
	pacer           *common.Pacer

	debug                 bool
	lastAckPrintTimestamp int64
	mqManager             *QuicMqManager
	actionMap             map[int]int
	cwnd                  congestion.ByteCount
	ctx                   context.Context
}

func NewRLSender(ctx context.Context, redisConf *RedisConf) *RLSender {
	debug, _ := strconv.ParseBool(os.Getenv(debugEnv))
	bs := &RLSender{
		maxDatagramSize: congestion.InitialPacketSizeIPv4,
		debug:           debug,
		cwnd:            initialCongestionWindow * congestion.InitialPacketSizeIPv4,
		ctx:             ctx,
	}
	bs.pacer = common.NewPacer(func() congestion.ByteCount {
		return bs.cwnd
	})
	if redisConf == nil {
		panic("Empty redis conf")
	}

	//init mq
	r, err := NewRedisManager(redisConf.Host, redisConf.Port, "")
	if err != nil {
		fmt.Println("Failed to create Redis manager:", err)
		panic(err)
	}

	bs.mqManager = NewQuicMqManager(r, MQ_CONNECTION_ID)
	bs.actionMap = map[int]int{
		0: -3,
		1: -1,
		2: 0,
		3: 1,
		4: 3,
	}
	// start mq
	actionCh := bs.mqManager.GetActionCh()
	// quic listen action
	go bs.mqManager.ListenAction(bs.ctx)
	// apply action to cwnd
	go func() {
		for action := range actionCh {
			fmt.Println("quic: apply action", action)
			bs.cwnd += congestion.ByteCount(bs.actionMap[action.Action]) * bs.maxDatagramSize
			if bs.cwnd < 0 {
				bs.cwnd = 10240
			}
			fmt.Println("quic: new cwnd", bs.cwnd)
		}
	}()
	// publish states
	go func() {
		for {
			select {
			case <-bs.ctx.Done():
				fmt.Println(bs.ctx.Err())
				err = bs.mqManager.PublishState(&StateMsg{
					FIN: true,
				})
				if err != nil {
					fmt.Println(err)
				}
				return
			case <-time.Tick(time.Second):
				if bs.rttStats == nil {
					continue
				}
				rtt := bs.rttStats.SmoothedRTT()
				msg := StateMsg{
					Cwnd: bs.cwnd * bs.maxDatagramSize,
					Rtt:  rtt.Microseconds(),
				}
				err = bs.mqManager.PublishState(&msg)
				if err != nil {
					fmt.Println(err)
				}
				msg_bytes, _ := json.Marshal(msg)
				fmt.Println("quic: publish state", string(msg_bytes))
			}
		}
	}()
	return bs
}

func (b *RLSender) SetRTTStatsProvider(rttStats congestion.RTTStatsProvider) {
	b.rttStats = rttStats

}

func (b *RLSender) TimeUntilSend(bytesInFlight congestion.ByteCount) time.Time {
	return b.pacer.TimeUntilSend()
}

func (b *RLSender) HasPacingBudget(now time.Time) bool {
	return b.pacer.Budget(now) >= b.maxDatagramSize
}

func (b *RLSender) CanSend(bytesInFlight congestion.ByteCount) bool {
	return bytesInFlight < b.GetCongestionWindow()
}

func (b *RLSender) GetCongestionWindow() congestion.ByteCount {
	return b.cwnd
}

func (b *RLSender) OnPacketSent(sentTime time.Time, bytesInFlight congestion.ByteCount,
	packetNumber congestion.PacketNumber, bytes congestion.ByteCount, isRetransmittable bool,
) {
	b.pacer.SentPacket(sentTime, bytes)
}

func (b *RLSender) OnPacketAcked(number congestion.PacketNumber, ackedBytes congestion.ByteCount,
	priorInFlight congestion.ByteCount, eventTime time.Time,
) {
	// Stub
}

func (b *RLSender) OnCongestionEvent(number congestion.PacketNumber, lostBytes congestion.ByteCount,
	priorInFlight congestion.ByteCount,
) {
	// Stub
}

func (b *RLSender) OnCongestionEventEx(priorInFlight congestion.ByteCount, eventTime time.Time, ackedPackets []congestion.AckedPacketInfo, lostPackets []congestion.LostPacketInfo) {

}

func (b *RLSender) SetMaxDatagramSize(size congestion.ByteCount) {
	b.maxDatagramSize = size
	b.pacer.SetMaxDatagramSize(size)
	if b.debug {
		b.debugPrint("SetMaxDatagramSize: %d", size)
	}
}

func (b *RLSender) InSlowStart() bool {
	return false
}

func (b *RLSender) InRecovery() bool {
	return false
}

func (b *RLSender) MaybeExitSlowStart() {}

func (b *RLSender) OnRetransmissionTimeout(packetsRetransmitted bool) {}

func (b *RLSender) canPrintAckRate(currentTimestamp int64) bool {
	return b.debug && currentTimestamp-b.lastAckPrintTimestamp >= debugPrintInterval
}

func (b *RLSender) debugPrint(format string, a ...any) {
	fmt.Printf("[BrutalSender] [%s] %s\n",
		time.Now().Format("15:04:05"),
		fmt.Sprintf(format, a...))
}
