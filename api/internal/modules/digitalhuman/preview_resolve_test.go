package digitalhuman

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
)

type previewBindingReader struct{ resourcebindings.Reader }

// A preview runs the draft without memory, the Linux sandbox, or connector
// actions, while published runs keep their snapshot unchanged.
func TestPreviewSnapshotTurnsOffSideEffects(t *testing.T) {
	adapter := NewBrainAdapter(previewBindingReader{})
	resolved := &ResolvedExecution{Snapshot: ExecutionSnapshot{IdentityName: "Ada", Model: "m", Revision: "r", Phase: string(resourcebindings.PhaseDraft)}}
	resolved.Snapshot.Subject.ID = resolved.Snapshot.DigitalHumanVersion
	resolved.Snapshot.Subject.ID[0] = 1
	config, err := adapter.RunConfig(resolved)
	if err != nil {
		t.Fatalf("RunConfig() error = %v", err)
	}
	if !config.ConnectorToolsDisabled || config.ConnectorTargetScope != "draft" || config.BindingSnapshot.Phase != resourcebindings.PhaseDraft {
		t.Fatalf("preview config = connectors disabled %v, scope %q, phase %q", config.ConnectorToolsDisabled, config.ConnectorTargetScope, config.BindingSnapshot.Phase)
	}
	resolved.Snapshot.Phase = ""
	config, err = adapter.RunConfig(resolved)
	if err != nil || config.ConnectorToolsDisabled || config.ConnectorTargetScope != "published" {
		t.Fatalf("published config = %#v, %v", config.ConnectorTargetScope, err)
	}
	if !previewRun(&Run{Surface: SurfacePreview}) || previewRun(&Run{Surface: SurfaceConsole}) {
		t.Fatal("previewRun misclassifies surfaces")
	}
	if got := digitalHumanConversationSourceForSurface(SurfacePreview); got != "preview" {
		t.Fatalf("preview conversation source = %q", got)
	}
}
