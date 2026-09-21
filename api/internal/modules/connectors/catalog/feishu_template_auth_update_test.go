package catalog

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func feishu102TemplateFixture(t *testing.T) Definition {
	t.Helper()
	d := feishu110Fixture(t)
	d.PluginVersion = "1.0.2"
	d.Operations = slices.DeleteFunc(d.Operations, func(op Operation) bool { return op.ID == "feishu.contact.search" })
	d.Setup.TemplateID = "feishu-app-template-v2"
	d.Setup.TemplateDigest = "4930f0a57d31ed17ed4d86384fd05219cfce7c5b4f20bbfcadec2329307634cc"
	digest, err := DefinitionContractDigest(d)
	if err != nil || digest != "8313044f4f770570c1e567ad24af7f30b7b497ae6fa8b19b2eacf3d8a405c430" {
		t.Fatalf("accepted 1.0.2 fixture drifted: %s, %v", digest, err)
	}
	return d
}

func TestFeishuTemplateAuthenticationMigrationAcrossReleases(t *testing.T) {
	current := feishu102TemplateFixture(t)
	latest, err := FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	oldAuth, _ := digestJSON(canonicalAuthentication(current))
	newAuth, _ := digestJSON(canonicalAuthentication(latest))
	if oldAuth != "e8eb91b5a336036bea786350d1062036efe15faf9c933966c4674c4159fc8063" ||
		newAuth != "fe5d0bc4b8c79a75eb673839e88a6b4de9ab65582b876228bdb95463ed0b74c4" {
		t.Fatalf("reviewed authentication drifted: old=%s new=%s", oldAuth, newAuth)
	}
	for _, version := range []string{"1.0.4", "1.2.0-digitalhuman.1", "1.3.0"} {
		t.Run(version, func(t *testing.T) {
			candidate := cloneDefinition(latest)
			candidate.PluginVersion = version
			// A newly admitted action is independent of the one-time auth migration.
			additional := cloneDefinition(latest).Operations[0]
			additional.ID = "feishu.bot.inspect"
			additional.ToolName = "feishu_bot_inspect"
			candidate.Operations = append(candidate.Operations, additional)
			if err := CheckCompatibleUpdate(current, candidate); err != nil {
				t.Fatalf("reviewed authentication with additive release rejected: %v", err)
			}
			assertFeishuTemplateBootstrap(t, current, candidate)
		})
	}
	t.Run("old template with a maintenance release", func(t *testing.T) {
		prior := cloneDefinition(current)
		prior.PluginVersion = "1.0.2-maintenance.1"
		if err := CheckCompatibleUpdate(prior, latest); err != nil {
			t.Fatalf("reviewed source authentication should not depend on its release label: %v", err)
		}
	})
	t.Run("canonical ordering and display copy", func(t *testing.T) {
		prior, target := cloneDefinition(current), cloneDefinition(latest)
		slices.Reverse(prior.CredentialFields)
		slices.Reverse(prior.Setup.Methods)
		slices.Reverse(target.CredentialFields)
		slices.Reverse(target.Setup.Methods)
		target.CredentialFields[0].Label = "Updated display label"
		beforePrior, beforeTarget := cloneDefinition(prior), cloneDefinition(target)
		if err := CheckCompatibleUpdate(prior, target); err != nil {
			t.Fatalf("equivalent authentication ordering/display rejected: %v", err)
		}
		if !reflect.DeepEqual(prior, beforePrior) || !reflect.DeepEqual(target, beforeTarget) {
			t.Fatal("authentication canonicalization mutated input")
		}
	})
}

func TestFeishuTemplateAuthenticationMigrationBoundaries(t *testing.T) {
	current := feishu102TemplateFixture(t)
	candidate, err := FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	for side, base := range map[string]Definition{"source": current, "target": candidate} {
		for name, mutate := range map[string]func(*Definition){
			"unknown template":    func(d *Definition) { d.Setup.TemplateID = "feishu-app-template-unknown" },
			"template digest":     func(d *Definition) { d.Setup.TemplateDigest = strings.Repeat("f", 64) },
			"credential secret":   func(d *Definition) { d.CredentialFields[0].Secret = !d.CredentialFields[0].Secret },
			"credential name":     func(d *Definition) { d.CredentialFields[0].Name = "other" },
			"credential length":   func(d *Definition) { d.CredentialFields[0].MaxLength++ },
			"credential required": func(d *Definition) { d.CredentialFields[0].Required = false },
			"credential type":     func(d *Definition) { d.CredentialFields[0].Type = "object" },
			"credential enum":     func(d *Definition) { d.CredentialFields[0].Enum = []string{"other"} },
			"setup handler":       func(d *Definition) { d.Setup.Handler = "feishu-app-registration-v2" },
			"setup methods":       func(d *Definition) { d.Setup.Methods = []string{"manual"} },
			"setup protocol":      func(d *Definition) { d.Setup.ProtocolVersion = 1 },
			"setup intents":       func(d *Definition) { d.Setup.Intents = []string{"connect"} },
			"authentication kind": func(d *Definition) { d.Setup.AuthenticationKind = "user" },
			"verification policy": func(d *Definition) { d.Setup.VerificationPolicy = "none" },
			"contract version":    func(d *Definition) { d.ContractVersion++ },
			"extension schema":    func(d *Definition) { d.ExtensionSchemaVersion = "9.0.0" },
		} {
			t.Run(side+"/"+name, func(t *testing.T) {
				changed := cloneDefinition(base)
				mutate(&changed)
				left, right := current, candidate
				if side == "source" {
					left = changed
				} else {
					right = changed
				}
				if reviewedFeishuTemplateExpansion(left, right) || CheckCompatibleUpdate(left, right) == nil {
					t.Fatal("unreviewed authentication was accepted")
				}
			})
		}
	}
	for name, mutate := range map[string]func(*Definition){
		"same version":   func(d *Definition) { d.PluginVersion = current.PluginVersion },
		"source dialect": func(d *Definition) { d.SourceDialect = "other" },
		"source schema":  func(d *Definition) { d.SourceSchemaVersion = "9.0.0" },
		"removed old operation": func(d *Definition) {
			d.Operations = slices.DeleteFunc(d.Operations, func(op Operation) bool { return op.ID == current.Operations[0].ID })
		},
		"changed old input":  func(d *Definition) { d.Operations[0].InputSchema["maxProperties"] = 0 },
		"changed old output": func(d *Definition) { d.Operations[0].OutputSchema["maxProperties"] = 0 },
		"changed old risk":   func(d *Definition) { d.Operations[0].RiskHint = OperationRiskHintCritical },
		"changed old scopes": func(d *Definition) { d.Operations[0].RequiredScopes = [][]string{{"tenant:tenant:readonly"}} },
	} {
		t.Run(name, func(t *testing.T) {
			changed := cloneDefinition(candidate)
			mutate(&changed)
			if CheckCompatibleUpdate(current, changed) == nil {
				t.Fatal("other compatibility restrictions were bypassed")
			}
		})
	}
	if reviewedFeishuTemplateExpansion(candidate, current) || CheckCompatibleUpdate(candidate, current) == nil {
		t.Fatal("template rollback accepted")
	}
	originalCurrent, originalCandidate := cloneDefinition(current), cloneDefinition(candidate)
	if err := CheckCompatibleUpdate(current, candidate); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(current, originalCurrent) || !reflect.DeepEqual(candidate, originalCandidate) {
		t.Fatal("compatibility check mutated input definitions")
	}
}

func TestFeishuSameTemplateUsesOrdinaryCompatibility(t *testing.T) {
	current := feishu110Fixture(t)
	candidate, err := FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	candidate.PluginVersion = "1.4.0-maintenance.1"
	if reviewedFeishuTemplateExpansion(current, candidate) {
		t.Fatal("same-template release must not use the template transition")
	}
	if err := CheckCompatibleUpdate(current, candidate); err != nil {
		t.Fatalf("same-template additive release rejected: %v", err)
	}
}
