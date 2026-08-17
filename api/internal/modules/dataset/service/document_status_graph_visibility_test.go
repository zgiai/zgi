package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
)

type fakeDocumentVisibilityWriter struct {
	calls   int
	request graphflow.VisibilityChangeRequest
}

func (f *fakeDocumentVisibilityWriter) SetDocumentRetrievalEnabled(
	ctx context.Context,
	request graphflow.VisibilityChangeRequest,
) (int64, bool, error) {
	f.calls++
	f.request = request
	return 1, true, nil
}

func TestSyncDocumentGraphVisibilityUsesStableRefWithoutGraphRun(t *testing.T) {
	writer := &fakeDocumentVisibilityWriter{}
	organizationID := uuid.New()
	workspaceID := uuid.New()
	datasetID := uuid.New()
	documentID := uuid.New()
	refID := uuid.New()

	result, err := syncDocumentGraphVisibility(context.Background(), writer, graphflow.VisibilityChangeRequest{
		OrganizationID:   organizationID,
		WorkspaceID:      &workspaceID,
		DatasetID:        datasetID,
		SourceRefID:      refID,
		DocumentID:       documentID,
		RetrievalEnabled: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if writer.calls != 1 || result.RunCreated {
		t.Fatalf("visibility calls=%d run_created=%v", writer.calls, result.RunCreated)
	}
	if writer.request.SourceRefID != refID || writer.request.DocumentID != documentID {
		t.Fatalf("visibility request=%#v", writer.request)
	}
}
