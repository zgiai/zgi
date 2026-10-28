// This file matches an organization's command rules (ADR 0029). A rule names
// a command prefix and an action: forbid refuses the command, ask makes the
// person approve it even where it would otherwise run without asking. Rules
// only tighten; they never let a command run that would otherwise ask.
package serverops

import (
	"fmt"
	"path"
	"strings"
	"unicode"
)

// RuleAction is what a matching rule does.
type RuleAction string

const (
	// RuleNone means no rule matched.
	RuleNone RuleAction = ""
	// RuleAsk makes the person approve the call.
	RuleAsk RuleAction = "ask"
	// RuleForbid refuses the call before approval.
	RuleForbid RuleAction = "forbid"
)

// Bounds of one rule prefix.
const (
	maxRulePrefixBytes  = 200
	maxRulePrefixTokens = 12
)

// CommandRule is one organization rule.
type CommandRule struct {
	Prefix string
	Action RuleAction
}

// NormalizeRulePrefix validates a rule prefix and returns it with single
// spaces between its words.
func NormalizeRulePrefix(prefix string) (string, error) {
	tokens := strings.Fields(prefix)
	if len(tokens) == 0 || len(tokens) > maxRulePrefixTokens {
		return "", fmt.Errorf("%w: a rule needs 1 to %d words", ErrInvalidInput, maxRulePrefixTokens)
	}
	for _, token := range tokens {
		for _, r := range token {
			if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
				return "", fmt.Errorf("%w: a rule cannot contain control characters", ErrInvalidInput)
			}
		}
	}
	normalized := strings.Join(tokens, " ")
	if len(normalized) > maxRulePrefixBytes {
		return "", fmt.Errorf("%w: a rule is longer than %d bytes", ErrInvalidInput, maxRulePrefixBytes)
	}
	return normalized, nil
}

// MatchCommandRules returns the strongest action of the rules that match one
// call: forbid over ask over none. A shell command is matched on each of its
// simple commands; a command the grammar cannot parse is matched on its raw
// words, which may over-match but never under-matches. The fixed service
// actions are matched as the systemctl or journalctl command they run.
func MatchCommandRules(action string, input map[string]any, rules []CommandRule) RuleAction {
	if len(rules) == 0 {
		return RuleNone
	}
	commands, raw := ruleCommandWords(action, input)
	strongest := RuleNone
	for _, rule := range rules {
		prefix := strings.Fields(rule.Prefix)
		if len(prefix) == 0 || (rule.Action != RuleAsk && rule.Action != RuleForbid) {
			continue
		}
		matched := raw != nil && containsTokens(raw, prefix)
		for _, words := range commands {
			if matched {
				break
			}
			matched = prefixMatches(words, prefix)
		}
		if !matched {
			continue
		}
		if rule.Action == RuleForbid {
			return RuleForbid
		}
		strongest = RuleAsk
	}
	return strongest
}

// ruleCommandWords returns the simple commands a call runs, or the raw words
// of a command the grammar cannot parse.
func ruleCommandWords(action string, input map[string]any) ([][]string, []string) {
	unit, _ := input["service_name"].(string)
	switch action {
	case ActionServiceRestart, ActionServiceStart, ActionServiceStop, ActionServiceReload:
		verb := strings.TrimPrefix(action, "server.service.")
		return [][]string{{"systemctl", verb, unit}}, nil
	case ActionServiceStatus:
		return [][]string{{"systemctl", "status", unit}}, nil
	case ActionJournalTail:
		return [][]string{{"journalctl", "-u", unit}}, nil
	case ActionCommandRun:
	default:
		return nil, nil
	}
	command, _ := input["command"].(string)
	parsed, ok := parseCommandLine(command)
	if !ok {
		return nil, strings.FieldsFunc(command, func(r rune) bool {
			return unicode.IsSpace(r) || strings.ContainsRune(";&|()`$<>'\"{}\\", r)
		})
	}
	commands := make([][]string, 0, len(parsed))
	for _, simple := range parsed {
		commands = append(commands, unwrapCommand(simple.words))
	}
	return commands, nil
}

// unwrapCommand drops wrappers that run another command, so a rule on that
// command also matches it through sudo, command, exec, nohup, or env.
func unwrapCommand(words []string) []string {
	for len(words) > 0 {
		name, _ := programName(words[0])
		switch name {
		case "sudo", "command", "exec", "nohup", "env":
			rest := words[1:]
			for len(rest) > 0 && (strings.HasPrefix(rest[0], "-") || strings.Contains(rest[0], "=")) {
				if rest[0] == "-u" || rest[0] == "-g" {
					if len(rest) < 2 {
						return nil
					}
					rest = rest[1:]
				}
				rest = rest[1:]
			}
			words = rest
		default:
			return words
		}
	}
	return words
}

// prefixMatches reports whether a simple command runs the rule's program and
// carries the rest of its words in order, flags in between allowed.
func prefixMatches(words, prefix []string) bool {
	if len(words) == 0 || !ruleWordMatches(path.Base(words[0]), prefix[0]) {
		return false
	}
	next := 1
	for _, word := range words[1:] {
		if next < len(prefix) && ruleWordMatches(word, prefix[next]) {
			next++
		}
	}
	return next == len(prefix)
}

// containsTokens reports whether the rule's words appear in order in a raw
// command.
func containsTokens(words, prefix []string) bool {
	next := 0
	for _, word := range words {
		if next < len(prefix) && ruleWordMatches(path.Base(word), prefix[next]) || next < len(prefix) && ruleWordMatches(word, prefix[next]) {
			next++
		}
	}
	return next == len(prefix)
}

// ruleWordMatches compares one word with a rule word; a unit name matches
// with or without its .service suffix.
func ruleWordMatches(word, rule string) bool {
	return word == rule || word == rule+".service" || strings.TrimSuffix(word, ".service") == rule
}
