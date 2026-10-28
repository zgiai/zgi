package serveropsssh

import "github.com/zgiai/zgi/api/internal/capabilities/serverops"

// WatchConnection builds the pinned connection a server watch reads service
// states through (ADR 0031). The caller must invoke the release function after
// the capability calls return.
func WatchConnection(values map[string]string) (serverops.Connection, func(), error) {
	return connectionFrom(values)
}
