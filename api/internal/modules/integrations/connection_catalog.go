package integrations

import "strings"

type ConnectionCatalog interface {
	DriverForIntegration(integrationID string) (string, bool)
	Actions(integrationID string) []ActionDefinition
}

type ConnectionAuthMethodCatalog interface {
	ResolveConnectionAuthMethod(integrationID, authMethodID string, authType ConnectionAuthType) (AuthMethodDefinition, bool)
}

func (registry *Registry) DriverForIntegration(integrationID string) (string, bool) {
	registration, exists := registry.Registration(strings.ToLower(strings.TrimSpace(integrationID)))
	if !exists {
		return "", false
	}
	driverID := strings.ToLower(strings.TrimSpace(registration.Definition.DriverID))
	return driverID, driverID != ""
}

func (registry *Registry) ResolveConnectionAuthMethod(integrationID, authMethodID string, authType ConnectionAuthType) (AuthMethodDefinition, bool) {
	registration, exists := registry.Registration(strings.ToLower(strings.TrimSpace(integrationID)))
	if !exists {
		return AuthMethodDefinition{}, false
	}
	authMethodID = strings.ToLower(strings.TrimSpace(authMethodID))
	wantedType := AuthMethodType(strings.ToLower(strings.TrimSpace(string(authType))))
	var matching []AuthMethodDefinition
	for _, method := range registration.Definition.AuthMethods {
		if !method.Available {
			if authMethodID != "" && method.ID == authMethodID {
				return AuthMethodDefinition{}, false
			}
			continue
		}
		if authMethodID != "" && method.ID == authMethodID {
			return method, method.Type == wantedType
		}
		if method.Type == wantedType {
			matching = append(matching, method)
		}
	}
	if authMethodID != "" || len(matching) != 1 {
		return AuthMethodDefinition{}, false
	}
	return matching[0], true
}
