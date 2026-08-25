package skills

import "testing"

func TestProjectedExternalActionOperationItemIDIsStableAndPhaseBound(t *testing.T) {
	first := ProjectedExternalActionOperationItemID("phase-first", "epoch-1", "binding-1", "WeCom", "wecom.message.send", "CONNECTION-1")
	replay := ProjectedExternalActionOperationItemID("phase-first", "epoch-1", "binding-1", "wecom", "wecom.message.send", "connection-1")
	second := ProjectedExternalActionOperationItemID("phase-second", "epoch-2", "binding-1", "wecom", "wecom.message.send", "connection-1")
	if first == "" || replay != first {
		t.Fatalf("stable phase identity first=%q replay=%q", first, replay)
	}
	if second == "" || second == first {
		t.Fatalf("distinct phases shared identity first=%q second=%q", first, second)
	}
	if got := ProjectedExternalActionOperationItemID("phase-first", "", "binding-1", "wecom", "wecom.message.send", "connection-1"); got != "" {
		t.Fatalf("incomplete server binding produced identity %q", got)
	}
}

func TestExternalActionOperationItemIDRuntimeParameterRejectsSpoofedValues(t *testing.T) {
	trusted := ProjectedExternalActionOperationItemID("phase-first", "epoch-1", "binding-1", "wecom", "wecom.message.send", "connection-1")
	parameters := map[string]interface{}{externalActionOperationItemIDRuntimeParameter: "phase:attacker", "other": "preserved"}
	bound := WithExternalActionOperationItemID(parameters, trusted)
	if got := ExternalActionOperationItemIDFromRuntimeParameters(bound); got != trusted {
		t.Fatalf("runtime operation identity = %q, want %q", got, trusted)
	}
	if got := ExternalActionOperationItemIDFromRuntimeParameters(parameters); got != "" {
		t.Fatalf("malformed injected identity was accepted: %q", got)
	}
	if parameters[externalActionOperationItemIDRuntimeParameter] != "phase:attacker" || bound["other"] != "preserved" {
		t.Fatalf("runtime parameter copy mutated source or lost values: source=%#v bound=%#v", parameters, bound)
	}
	parameters[externalActionOperationItemIDRuntimeParameter] = trusted
	if got := ExternalActionOperationItemIDFromRuntimeParameters(parameters); got != "" {
		t.Fatalf("well-formed plain-string spoof was accepted: %q", got)
	}
}
