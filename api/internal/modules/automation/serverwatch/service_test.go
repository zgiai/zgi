package serverwatch

import (
	"testing"
	"time"

	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

func TestEvaluateRaisesOneAlertPerOutage(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	services := []string{"nginx", "redis"}
	down := Observation{Services: map[string]string{"nginx": "failed", "redis": "active"}}

	state, alerts := Evaluate(nil, services, down, 2, now)
	if len(alerts) != 0 || state["nginx"].Failures != 1 || state["nginx"].Healthy {
		t.Fatalf("first failure: alerts=%v state=%+v", alerts, state["nginx"])
	}
	state, alerts = Evaluate(state, services, down, 2, now.Add(time.Minute))
	if len(alerts) != 1 || alerts[0]["name"] != "nginx" || alerts[0]["observed"] != "failed" {
		t.Fatalf("threshold reached: alerts=%v", alerts)
	}
	// The sweep marks it alerted once the task started.
	target := state["nginx"]
	target.Alerted = true
	state["nginx"] = target
	state, alerts = Evaluate(state, services, down, 2, now.Add(2*time.Minute))
	if len(alerts) != 0 {
		t.Fatalf("same outage raised again: %v", alerts)
	}
	up := Observation{Services: map[string]string{"nginx": "active", "redis": "active"}}
	state, _ = Evaluate(state, services, up, 2, now.Add(3*time.Minute))
	if !state["nginx"].Healthy || state["nginx"].Alerted || state["nginx"].Failures != 0 {
		t.Fatalf("recovery did not reset: %+v", state["nginx"])
	}
	state, _ = Evaluate(state, services, down, 2, now.Add(4*time.Minute))
	_, alerts = Evaluate(state, services, down, 2, now.Add(5*time.Minute))
	if len(alerts) != 1 {
		t.Fatalf("next outage not raised: %v", alerts)
	}
}

func TestEvaluateHostDownDoesNotJudgeServices(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	state, alerts := Evaluate(nil, []string{"nginx"}, Observation{HostError: "unreachable"}, 1, now)
	if len(alerts) != 1 || alerts[0]["name"] != digitalhuman.ServerWatchHostKey {
		t.Fatalf("host alert: %v", alerts)
	}
	if _, judged := state["nginx"]; judged {
		t.Fatal("a service was judged while the host was down")
	}
}

func TestEvaluateDropsServicesNoLongerWatched(t *testing.T) {
	previous := map[string]digitalhuman.ServerWatchTarget{"old": {Healthy: false, Failures: 3}}
	state, _ := Evaluate(previous, []string{"nginx"}, Observation{Services: map[string]string{"nginx": "active"}}, 2, time.Now())
	if _, kept := state["old"]; kept {
		t.Fatal("state kept a service that is no longer watched")
	}
}

func TestNormalizeServicesRejectsShellSyntax(t *testing.T) {
	if got, err := normalizeServices([]string{" nginx ", "nginx", "redis.service"}); err != nil || len(got) != 2 {
		t.Fatalf("normalizeServices() = %v, %v", got, err)
	}
	for _, bad := range [][]string{nil, {"a;reboot"}, {"-h"}, {"a b"}, {"$(id)"}} {
		if _, err := normalizeServices(bad); err == nil {
			t.Fatalf("normalizeServices(%q) accepted", bad)
		}
	}
	many := make([]string, maxServices+1)
	for i := range many {
		many[i] = "svc" + string(rune('a'+i))
	}
	if _, err := normalizeServices(many); err == nil {
		t.Fatal("more than the limit accepted")
	}
}
