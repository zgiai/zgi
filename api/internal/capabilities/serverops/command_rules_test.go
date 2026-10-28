package serverops

import (
	"errors"
	"strings"
	"testing"
)

func TestMatchCommandRules(t *testing.T) {
	rules := []CommandRule{
		{Prefix: "git push", Action: RuleForbid},
		{Prefix: "systemctl restart nginx", Action: RuleAsk},
		{Prefix: "docker rm", Action: RuleAsk},
		{Prefix: "docker rm -f", Action: RuleForbid},
	}
	command := func(value string) map[string]any { return map[string]any{"command": value} }
	for name, test := range map[string]struct {
		action string
		input  map[string]any
		want   RuleAction
	}{
		"exact":                {ActionCommandRun, command("git push origin main"), RuleForbid},
		"flags between":        {ActionCommandRun, command("git -C /srv/app push --force"), RuleForbid},
		"later in a chain":     {ActionCommandRun, command("cd /srv/app && git status && git push"), RuleForbid},
		"through sudo":         {ActionCommandRun, command("sudo -u deploy git push"), RuleForbid},
		"absolute program":     {ActionCommandRun, command("/usr/bin/git push"), RuleForbid},
		"unparsed raw words":   {ActionCommandRun, command("cd $APP && git push"), RuleForbid},
		"unit suffix":          {ActionCommandRun, command("systemctl restart nginx.service"), RuleAsk},
		"service action":       {ActionServiceRestart, map[string]any{"service_name": "nginx.service"}, RuleAsk},
		"forbid beats ask":     {ActionCommandRun, command("docker rm -f web"), RuleForbid},
		"ask only":             {ActionCommandRun, command("docker rm web"), RuleAsk},
		"other subcommand":     {ActionCommandRun, command("git pull --ff-only"), RuleNone},
		"other unit":           {ActionServiceRestart, map[string]any{"service_name": "app.service"}, RuleNone},
		"argument is not run":  {ActionCommandRun, command("grep 'git push' notes.txt"), RuleNone},
		"snapshot never rules": {ActionSystemSnapshot, map[string]any{}, RuleNone},
	} {
		if got := MatchCommandRules(test.action, test.input, rules); got != test.want {
			t.Errorf("%s: MatchCommandRules() = %q, want %q", name, got, test.want)
		}
	}
	if MatchCommandRules(ActionCommandRun, command("git push"), nil) != RuleNone {
		t.Error("no rules matched something")
	}
}

func TestNormalizeRulePrefix(t *testing.T) {
	if got, err := NormalizeRulePrefix("  systemctl   restart\tnginx "); err != nil || got != "systemctl restart nginx" {
		t.Fatalf("NormalizeRulePrefix() = %q, %v", got, err)
	}
	for _, bad := range []string{"", "   ", "rm\x00 -rf", strings.Repeat("a ", 13), strings.Repeat("x", 201)} {
		if _, err := NormalizeRulePrefix(bad); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("NormalizeRulePrefix(%q) error = %v", bad, err)
		}
	}
}
