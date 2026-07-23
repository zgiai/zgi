package skillloop

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

var skillLoadAttemptSequence atomic.Uint64

func newSkillLoadAttemptRuntimeID(skillID string) string {
	return fmt.Sprintf(
		"skill_load_attempt:%d:%d:%s",
		time.Now().UnixNano(),
		skillLoadAttemptSequence.Add(1),
		strings.ToLower(strings.TrimSpace(skillID)),
	)
}
