package main

import (
	"strings"
	"testing"
)

const advancedCheckpointHeadID = "202609101110000000_add_digital_human_task_ownership"

// The advanced checkpoint is an explicit plan/apply opt-in. Presence is decided
// by flags.Visit, never by an empty value, so an explicit --advanced-checkpoint=
// is rejected instead of being mistaken for the default.
func TestConnectorFinalizeOptionsAdvancedCheckpointContract(t *testing.T) {
	// A normal non-empty opt-in is forwarded for plan and apply.
	for _, command := range []string{"plan", "apply"} {
		option, err := connectorFinalizeOptions(command, true, advancedCheckpointHeadID)
		if err != nil {
			t.Fatalf("%s --advanced-checkpoint = %v, want nil", command, err)
		}
		if option.AdvancedCheckpoint != advancedCheckpointHeadID {
			t.Fatalf("%s forwarded AdvancedCheckpoint = %q, want %q", command, option.AdvancedCheckpoint, advancedCheckpointHeadID)
		}
	}
	// An explicitly empty value is not the default: plan and apply reject it.
	for _, command := range []string{"plan", "apply"} {
		if _, err := connectorFinalizeOptions(command, true, ""); err == nil {
			t.Fatalf("%s accepted an explicitly empty --advanced-checkpoint", command)
		} else if !strings.Contains(err.Error(), "--advanced-checkpoint") {
			t.Fatalf("%s empty-value rejection does not name the flag: %v", command, err)
		}
	}
	// When the flag is absent, every command keeps the default.
	for _, command := range []string{"plan", "apply", "verify"} {
		option, err := connectorFinalizeOptions(command, false, "")
		if err != nil {
			t.Fatalf("%s without --advanced-checkpoint = %v, want nil", command, err)
		}
		if option.AdvancedCheckpoint != "" {
			t.Fatalf("%s default AdvancedCheckpoint = %q, want empty", command, option.AdvancedCheckpoint)
		}
	}
	// verify rejects the flag whenever it appears, including an empty value.
	for _, value := range []string{"", advancedCheckpointHeadID} {
		if _, err := connectorFinalizeOptions("verify", true, value); err == nil {
			t.Fatalf("verify accepted --advanced-checkpoint with value %q", value)
		} else if !strings.Contains(err.Error(), "--advanced-checkpoint") {
			t.Fatalf("verify rejection does not name the flag: %v", err)
		}
	}
}

// Every advanced-checkpoint rejection must happen before any database
// configuration is loaded, so it is observable without a real database.
func TestRunConnectorFinalizeRejectsAdvancedCheckpointBeforeDatabase(t *testing.T) {
	for _, testCase := range []struct {
		name string
		args []string
	}{
		{name: "verify_explicit_value", args: []string{"verify", "--advanced-checkpoint", advancedCheckpointHeadID}},
		{name: "verify_explicit_empty", args: []string{"verify", "--advanced-checkpoint="}},
		{name: "plan_explicit_empty", args: []string{"plan", "--advanced-checkpoint="}},
		{name: "apply_explicit_empty", args: []string{"apply", "--advanced-checkpoint="}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := runConnectorFinalize(testCase.args)
			if err == nil {
				t.Fatalf("%v was accepted", testCase.args)
			}
			if !strings.Contains(err.Error(), "--advanced-checkpoint") {
				t.Fatalf("rejection does not name the flag: %v", err)
			}
		})
	}
}
