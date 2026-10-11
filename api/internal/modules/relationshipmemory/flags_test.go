package relationshipmemory

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRelationshipMemoryFeatureFlagsAreFailClosed(t *testing.T) {
	t.Setenv("ZGI_RELATIONSHIP_MEMORY_ENABLED", "")
	t.Setenv("ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED", "")
	t.Setenv("ZGI_AGENT_MEMORY_AUTO_EXTRACTION_ENABLED", "")
	if RuntimeEnabled() {
		t.Fatal("relationship memory runtime must be disabled when unset")
	}
	if AutomaticExtractionEnabled() {
		t.Fatal("automatic extraction must be disabled when unset")
	}

	t.Setenv("ZGI_RELATIONSHIP_MEMORY_ENABLED", "true")
	t.Setenv("ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED", "true")
	if !RuntimeEnabled() || !AutomaticExtractionEnabled() {
		t.Fatal("explicit true feature flags were not enabled")
	}
}

func TestAutomaticExtractionAcceptsLegacyFlagForRollingUpgrade(t *testing.T) {
	t.Setenv("ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED", "")
	t.Setenv("ZGI_AGENT_MEMORY_AUTO_EXTRACTION_ENABLED", "true")
	if !AutomaticExtractionEnabled() {
		t.Fatal("legacy automatic extraction flag was not honored")
	}
}

func TestProfileWritesFailClosedWithRuntimeDisabled(t *testing.T) {
	t.Setenv("ZGI_RELATIONSHIP_MEMORY_ENABLED", "")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	if relationshipMemoryWritesEnabled(ctx) {
		t.Fatal("profile write gate allowed a write while runtime was disabled")
	}
	if recorder.Code != 400 || !strings.Contains(recorder.Body.String(), ErrorCodePolicyRejected) {
		t.Fatalf("response = %d %s, want stable policy rejection", recorder.Code, recorder.Body.String())
	}

	t.Setenv("ZGI_RELATIONSHIP_MEMORY_ENABLED", "true")
	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	if !relationshipMemoryWritesEnabled(ctx) {
		t.Fatal("profile write gate rejected an enabled runtime")
	}
}
