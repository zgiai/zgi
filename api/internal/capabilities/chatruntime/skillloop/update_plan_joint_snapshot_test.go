package skillloop

import "testing"

func TestJointPlanOutcomeAssociations(t *testing.T) {
	for _, mode := range []string{"ordered", "explicit", "unknown", "partial", "count-mismatch"} {
		t.Run(mode, func(t *testing.T) {
			phases := []map[string]interface{}{{"id": "a"}, {"id": "b"}}
			outcomes := []map[string]interface{}{{"id": "one"}, {"id": "two"}}
			switch mode {
			case "explicit":
				phases[0]["outcome_id"], phases[1]["outcome_id"] = "two", "one"
			case "unknown":
				phases[0]["outcome_id"], phases[1]["outcome_id"] = "other", "two"
			case "partial":
				phases[0]["outcome_id"] = "one"
			case "count-mismatch":
				phases = phases[:1]
			}
			err := bindJointPlanOutcomes(phases, outcomes)
			valid := mode == "ordered" || mode == "explicit"
			if (err == nil) != valid {
				t.Fatalf("%s: %v", mode, err)
			}
			if mode == "ordered" && phases[0]["outcome_id"] != "one" {
				t.Fatal("missing ordered association")
			}
		})
	}
}
