package common

import "github.com/apernet/quic-go"

// TODO IANA registration
const QperfALPN = "qperf"

const DefaultQperfServerPort = 18080

const RuntimeReachedErrorCode = quic.ApplicationErrorCode(0)

const (
	CC_CUBIC  = "cubic"
	CC_RL     = "rl"
	CC_BRUTAL = "brutal"
)
