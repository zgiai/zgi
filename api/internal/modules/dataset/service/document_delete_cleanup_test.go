package service

import (
	"context"
	"errors"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
)

type documentVectorCleanerCall struct {
	className  string
	fieldName  string
	fieldValue string
}

type documentVectorCleanerStub struct {
	calls      []documentVectorCleanerCall
	errByClass map[string]error
	errByField map[string]error
	errByValue map[string]error
}

func (s *documentVectorCleanerStub) DeleteObjectsByField(ctx context.Context, className, fieldName, fieldValue string) error {
	s.calls = append(s.calls, documentVectorCleanerCall{
		className:  className,
		fieldName:  fieldName,
		fieldValue: fieldValue,
	})
	if s.errByClass != nil {
		if err := s.errByClass[className]; err != nil {
			return err
		}
	}
	if s.errByField != nil {
		if err := s.errByField[fieldName]; err != nil {
			return err
		}
	}
	if s.errByValue != nil {
		if err := s.errByValue[fieldValue]; err != nil {
			return err
		}
	}
	return nil
}

func TestDeleteDocumentVectorsByDocumentIDDeletesSegmentAndQuestionClasses(t *testing.T) {
	dataset := &model.Dataset{
		ID:          "1acadaf3-d516-4855-b174-6d837ba241cc",
		WorkspaceID: "workspace-1",
	}
	cleaner := &documentVectorCleanerStub{}
	service := &DocumentServiceImpl{vectorCleaner: cleaner}

	if err := service.deleteDocumentVectorsByDocumentID(context.Background(), dataset, []string{"doc-1", "doc-1", " doc-2 "}); err != nil {
		t.Fatalf("deleteDocumentVectorsByDocumentID returned error: %v", err)
	}

	segmentClass := model.GenCollectionNameByID(dataset.ID)
	questionClass := model.GenQuestionCollectionNameByID(dataset.ID)
	want := []documentVectorCleanerCall{
		{className: segmentClass, fieldName: "document_id", fieldValue: "doc-1"},
		{className: questionClass, fieldName: "document_id", fieldValue: "doc-1"},
		{className: segmentClass, fieldName: "document_id", fieldValue: "doc-2"},
		{className: questionClass, fieldName: "document_id", fieldValue: "doc-2"},
	}

	if len(cleaner.calls) != len(want) {
		t.Fatalf("call count = %d, want %d: %#v", len(cleaner.calls), len(want), cleaner.calls)
	}
	for i := range want {
		if cleaner.calls[i] != want[i] {
			t.Fatalf("call[%d] = %#v, want %#v", i, cleaner.calls[i], want[i])
		}
	}
}

func TestDeleteDocumentVectorsByDocumentIDReturnsSegmentCleanupError(t *testing.T) {
	dataset := &model.Dataset{ID: "dataset-1", WorkspaceID: "workspace-1"}
	segmentClass := model.GenCollectionNameByID(dataset.ID)
	expectedErr := errors.New("weaviate unavailable")
	cleaner := &documentVectorCleanerStub{
		errByClass: map[string]error{segmentClass: expectedErr},
	}
	service := &DocumentServiceImpl{vectorCleaner: cleaner}

	err := service.deleteDocumentVectorsByDocumentID(context.Background(), dataset, []string{"doc-1"})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected wrapped segment cleanup error, got %v", err)
	}
	if len(cleaner.calls) != 1 {
		t.Fatalf("call count = %d, want 1", len(cleaner.calls))
	}
}

func TestDeleteDocumentVectorsByDocumentIDIgnoresQuestionCleanupError(t *testing.T) {
	dataset := &model.Dataset{ID: "dataset-1", WorkspaceID: "workspace-1"}
	questionClass := model.GenQuestionCollectionNameByID(dataset.ID)
	cleaner := &documentVectorCleanerStub{
		errByClass: map[string]error{questionClass: errors.New("question class missing")},
	}
	service := &DocumentServiceImpl{vectorCleaner: cleaner}

	if err := service.deleteDocumentVectorsByDocumentID(context.Background(), dataset, []string{"doc-1"}); err != nil {
		t.Fatalf("question cleanup error should not fail document cleanup: %v", err)
	}
	if len(cleaner.calls) != 2 {
		t.Fatalf("call count = %d, want 2", len(cleaner.calls))
	}
}
