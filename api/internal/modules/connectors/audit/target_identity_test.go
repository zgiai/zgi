package audit

import (
	"database/sql/driver"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestAuditRecordsUseTheSameCanonicalTargetIdentity(t *testing.T) {
	version := uuid.NewString()
	for name, target := range map[string]core.TargetRef{
		"chat default":       {Type: core.TargetTypeChat, ID: "conversation"},
		"chat live":          {Type: core.TargetTypeChat, ID: "conversation", Scope: core.ScopeLive},
		"agent default":      {Type: core.TargetTypeAgent, ID: "agent"},
		"agent draft":        {Type: core.TargetTypeAgent, ID: "agent", Scope: core.ScopeDraft},
		"agent published":    {Type: core.TargetTypeAgent, ID: "agent", Scope: core.ScopePublished, VersionID: version},
		"workflow default":   {Type: core.TargetTypeWorkflow, ID: "workflow", BindingInstanceID: "node"},
		"workflow draft":     {Type: core.TargetTypeWorkflow, ID: "workflow", Scope: core.ScopeDraft, BindingInstanceID: "node"},
		"workflow published": {Type: core.TargetTypeWorkflow, ID: "workflow", Scope: core.ScopePublished, VersionID: version, BindingInstanceID: "node"},
	} {
		t.Run(name, func(t *testing.T) {
			decision := submittedDecision(uuid.New(), uuid.New(), uuid.New(), uuid.NewString())
			decision.Target = &target
			outcome := outcomeForDecision(decision, core.OutcomeStatusSuccessful)
			d, err := newLifecycleDecisionRecord(decision)
			if err != nil {
				t.Fatal(err)
			}
			o, err := newOperationOutcomeRecord(outcome)
			if err != nil {
				t.Fatal(err)
			}
			if d.TargetScope == nil || *d.TargetScope != string(target.EffectiveScope()) || o.TargetScope != string(target.EffectiveScope()) {
				t.Fatal("audit scope differs from the invocation claim scope")
			}
			for _, field := range []string{"TargetVersionID", "BindingInstanceID"} {
				left, err := driver.DefaultParameterConverter.ConvertValue(reflect.ValueOf(d).FieldByName(field).Interface())
				if err != nil {
					t.Fatal(err)
				}
				right, err := driver.DefaultParameterConverter.ConvertValue(reflect.ValueOf(o).FieldByName(field).Interface())
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(left, right) {
					t.Fatalf("%s is encoded differently: decision=%#v outcome=%#v", field, left, right)
				}
				if field == "TargetVersionID" && target.VersionID == "" && right != "" {
					t.Fatal("absent version must match the NOT NULL claim/outcome representation")
				}
			}
			stored, err := decisionFromRecord(d)
			if err != nil || !outcomeMatchesSubmittedDecision(outcome, stored) {
				t.Fatal("roundtrip changed target identity", err)
			}
			storedOutcome, err := outcomeFromRecord(o)
			if err != nil || !storedOutcome.Target.SameIdentity(target) {
				t.Fatal("outcome roundtrip changed scope/version", err)
			}
			if !sameOutcomeAuthority(storedOutcome, outcome) {
				t.Fatal("convergence rejected the persisted canonical target")
			}
			other := outcome
			other.Target.Scope = core.ScopePublished
			other.Target.VersionID = uuid.NewString()
			if outcomeMatchesSubmittedDecision(other, stored) {
				t.Fatal("different published version accepted")
			}
		})
	}
}
