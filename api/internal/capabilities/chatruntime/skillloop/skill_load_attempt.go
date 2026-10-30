package skillloop

import (
	"sync/atomic"
)

var skillLoadAttemptSequence atomic.Uint64
