package fxapp

import "go.uber.org/fx"

// integrationCompletionTestLifecycle is shared by lifecycle tests that are
// kept in separate files so each worker can focus on its own behavior.
type integrationCompletionTestLifecycle struct {
	hooks []fx.Hook
}

func (l *integrationCompletionTestLifecycle) Append(hook fx.Hook) {
	l.hooks = append(l.hooks, hook)
}
