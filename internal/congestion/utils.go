package congestion

import (
	"github.com/apernet/quic-go"
	bbr2 "qperf-go/internal/congestion/bbr"
	"qperf-go/internal/congestion/brutal"
	"qperf-go/internal/congestion/rl"
)

func UseBBR(conn quic.Connection) {
	conn.SetCongestionControl(bbr2.NewBbrSender(
		bbr2.DefaultClock{},
		bbr2.GetInitialPacketSize(conn.RemoteAddr()),
	))
}

func UseBrutal(conn quic.Connection, tx uint64) {
	conn.SetCongestionControl(brutal.NewBrutalSender(tx))
}
func UseRL(conn quic.Connection, redisConf *rl.RedisConf) {
	conn.SetCongestionControl(rl.NewRLSender(conn.Context(), redisConf))
}
