package model

import "testing"

func TestNormalizeDatasetPermissionDefaultsToAllTeam(t *testing.T) {
	if got := NormalizeDatasetPermission(""); got != string(DatasetPermissionAllTeam) {
		t.Fatalf("NormalizeDatasetPermission(\"\") = %q, want %q", got, DatasetPermissionAllTeam)
	}
	if got := NormalizeDatasetPermission("  "); got != string(DatasetPermissionAllTeam) {
		t.Fatalf("NormalizeDatasetPermission(blank) = %q, want %q", got, DatasetPermissionAllTeam)
	}
}

func TestIsValidDatasetCreatePermission(t *testing.T) {
	tests := []struct {
		name       string
		permission string
		want       bool
	}{
		{name: "empty defaults to all team", permission: "", want: true},
		{name: "only me", permission: string(DatasetPermissionOnlyMe), want: true},
		{name: "all team", permission: string(DatasetPermissionAllTeam), want: true},
		{name: "legacy all team members is read only compatibility", permission: string(DatasetPermissionAllTeamMembers), want: false},
		{name: "unknown", permission: "unexpected", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidDatasetCreatePermission(tt.permission); got != tt.want {
				t.Fatalf("IsValidDatasetCreatePermission(%q) = %v, want %v", tt.permission, got, tt.want)
			}
		})
	}
}

func TestIsDatasetWorkspaceVisiblePermission(t *testing.T) {
	tests := []struct {
		name       string
		permission string
		want       bool
	}{
		{name: "all team", permission: string(DatasetPermissionAllTeam), want: true},
		{name: "legacy all team members", permission: string(DatasetPermissionAllTeamMembers), want: true},
		{name: "only me", permission: string(DatasetPermissionOnlyMe), want: false},
		{name: "unknown", permission: "unexpected", want: false},
		{name: "empty", permission: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDatasetWorkspaceVisiblePermission(tt.permission); got != tt.want {
				t.Fatalf("IsDatasetWorkspaceVisiblePermission(%q) = %v, want %v", tt.permission, got, tt.want)
			}
		})
	}
}
