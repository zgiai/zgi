package v1

import (
	"os"
	"strings"
	"testing"
)

func TestMusicRoutesRequireOrganizationButNotWorkspace(t *testing.T) {
	source, err := os.ReadFile("music_routes.go")
	if err != nil {
		t.Fatalf("read music_routes.go: %v", err)
	}

	text := string(source)
	if !strings.Contains(text, "group.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))") {
		t.Fatal("music routes must retain organization authentication")
	}
	if strings.Contains(text, "group.Use(middleware.CurrentWorkspaceRequired())") {
		t.Fatal("music routes must not require a workspace")
	}
}
