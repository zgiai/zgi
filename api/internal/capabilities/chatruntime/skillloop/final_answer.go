package skillloop

import ()

type finalAnswerSubmission struct {
	answer      string
	explanation string
	plan        []map[string]interface{}
	streamed    bool
	planWarning string
}
