package common

import (
	"github.com/apernet/quic-go/logging"
	"net"
)

type migrationTracer struct {
	logging.Tracer
	onMigration func(addr net.Addr)
}

// func NewMigrationTracer(onMigration func(addr net.Addr)) logging.Tracer {
// 	return &migrationTracer{
// 		onMigration: onMigration,
// 	}
// }

// func (a migrationTracer) TracerForConnection(ctx context.Context, p logging.Perspective, odcid logging.ConnectionID) logging.ConnectionTracer {
// 	return connectionTracer{
// 		onMigration: a.onMigration,
// 	}
// }

type connectionTracer struct {
	logging.ConnectionTracer
	onMigration func(addr net.Addr)
}

func (a connectionTracer) UpdatedPath(newRemote net.Addr) {
	a.onMigration(newRemote)
}
