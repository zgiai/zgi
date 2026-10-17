package connectorfinalize

import "testing"

const (
	advancedHeadID   = "202609101110000000_add_digital_human_task_ownership"
	postCheckpointID = "20260904160000_connector_pending_setups"
)

func TestPreRetirementCheckpoint(t *testing.T) {
	base := []string{DetachCheckpointID, "20260903100000_create_connector_chat_preferences", "20260903110000_create_connector_onboarding", CheckpointID}
	for _, test := range []struct {
		name string
		ids  []string
		ok   bool
	}{
		{"messaging", base, true},
		{"unrelated_chat_runtime_external_user", append(append([]string{}, base...), "20260903120000_add_chat_runtime_external_user"), true},
		{"unrelated_video", append(append([]string{}, base...), "20260903183000_add_video_runtime_task_poster_url"), true},
		{"missing_detach", base[1:], false},
		{"missing_messaging", base[:3], false},
		{"gap", []string{DetachCheckpointID, CheckpointID}, false},
		{"retired", append(append([]string{}, base...), MigrationID), false},
		{"unknown", append(append([]string{}, base...), "20260905120000_unknown"), false},
		{"duplicate", append(append([]string{}, base...), CheckpointID), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			checkpoint, err := preRetirementCheckpoint(test.ids)
			if (err == nil) != test.ok || (test.ok && checkpoint != CheckpointID) {
				t.Fatalf("unexpected checkpoint: %q, %v", checkpoint, err)
			}
		})
	}
}

func TestRetirementReportBindsMigrationHistory(t *testing.T) {
	before := Report{Version: ReportVersion, Checkpoint: CheckpointID, MigrationHistoryDigest: digest([]string{CheckpointID})}
	before.seal()
	after := before
	after.MigrationHistoryDigest = digest([]string{CheckpointID, "20260903183000_add_video_runtime_task_poster_url"})
	after.seal()
	if before.Digest == after.Digest {
		t.Fatal("a changed migration history reused the reviewed report")
	}
}

func TestInspectionCheckpointAllowsDiscardPastCheckpoint(t *testing.T) {
	base := []string{DetachCheckpointID, "20260903100000_create_connector_chat_preferences", "20260903110000_create_connector_onboarding", CheckpointID}
	withMarker := append(append([]string{}, base...), postCheckpointID)
	deferred := append(append([]string{}, withMarker...), advancedHeadID)

	if _, err := inspectionCheckpoint(Options{}, withMarker); err == nil {
		t.Fatal("plan inspection accepted a post-checkpoint migration without an explicit advanced checkpoint")
	}
	for name, ids := range map[string][]string{"compatibility_marker": withMarker, "deferred_history": deferred} {
		t.Run(name, func(t *testing.T) {
			checkpoint, err := inspectionCheckpoint(Options{allowDiscardInspect: true}, ids)
			if err != nil {
				t.Fatalf("discard inspection rejected history: %v", err)
			}
			if checkpoint != ids[len(ids)-1] {
				t.Fatalf("discard checkpoint = %q, want latest applied %q", checkpoint, ids[len(ids)-1])
			}
		})
	}
	if _, err := inspectionCheckpoint(Options{allowDiscardInspect: true}, nil); err == nil {
		t.Fatal("discard inspection accepted an empty migration history")
	}
}
