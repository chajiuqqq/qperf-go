package common

import "time"

type Http3States struct{
	StartTime time.Time
	EndTime time.Time
	TimeUsageMS int64
	URL string
	BodySizeByte int
}