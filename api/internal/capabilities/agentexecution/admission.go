package agentexecution

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// EntryAdmission is operator-owned and independent of application opt-in and
// resource authorization. Its zero value enables only the legacy Agent console.
type EntryAdmission struct {
	configured bool
	allowed    map[string]map[string]bool
}

// ParseEntryAdmission accepts application:entry -> organization IDs. No
// wildcard or client-controlled policy inheritance is supported. An explicit
// empty object disables all entries; malformed settings fail closed.
func ParseEntryAdmission(raw string) (EntryAdmission, error) {
	if strings.TrimSpace(raw) == "" {
		return EntryAdmission{}, nil
	}
	denied := EntryAdmission{configured: true}
	if len(raw) > 64<<10 {
		return denied, fmt.Errorf("execution entry allowlist exceeds its limit")
	}
	var entries map[string][]string
	if json.Unmarshal([]byte(raw), &entries) != nil || entries == nil {
		return denied, fmt.Errorf("execution entry allowlist must be an object")
	}
	policy := EntryAdmission{configured: true, allowed: make(map[string]map[string]bool, len(entries))}
	for key, organizations := range entries {
		application, entry, ok := strings.Cut(key, ":")
		if !ok || !validEntry(application, entry) {
			return denied, fmt.Errorf("execution entry allowlist contains an unsupported entry")
		}
		policy.allowed[key] = make(map[string]bool, len(organizations))
		for _, value := range organizations {
			id, err := uuid.Parse(value)
			if err != nil || id == uuid.Nil || id.String() != value || policy.allowed[key][value] {
				return denied, fmt.Errorf("execution entry allowlist requires unique canonical organization IDs")
			}
			policy.allowed[key][value] = true
		}
	}
	return policy, nil
}

func (policy EntryAdmission) Allows(organization, application, entry string) bool {
	if organization == "" || !validEntry(application, entry) {
		return false
	}
	if !policy.configured {
		return application == "agent" && entry == "console"
	}
	return policy.allowed[application+":"+entry][organization]
}

func validEntry(application, entry string) bool {
	if application == "digital_human" && (entry == "channel" || entry == "task") {
		return true
	}
	return (application == "agent" || application == "digital_human") &&
		(entry == "console" || entry == "webapp" || entry == "external-api")
}
