package skillloop

import "strings"

func runtimeFailureAnswer(reason string, step string) string {
	reason = strings.TrimSpace(reason)
	step = strings.TrimSpace(step)
	if reason == "" {
		reason = "执行未能完成。"
	}
	if step == "" {
		return reason
	}
	return step + " 未能完成：" + reason
}
