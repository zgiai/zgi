package service

import "testing"

func TestRuntimeAttachmentRefsExposeOnlyAuthorizedMetadata(t *testing.T) {
	prepared := &PreparedChat{parts: &chatRequestParts{Attachments: &attachmentBundle{Files: []attachmentFile{{
		ID: "file-1", Name: "report.csv", Size: 12, Content: "private file content",
	}}}}}
	refs := prepared.RuntimeAttachmentRefs()
	if len(refs) != 1 || refs[0].ID != "file-1" || refs[0].Name != "report.csv" || refs[0].Size != 12 {
		t.Fatalf("attachment refs = %#v", refs)
	}
	refs[0].Name = "changed"
	if prepared.parts.Attachments.Files[0].Name != "report.csv" {
		t.Fatal("caller mutated prepared attachment metadata")
	}
	if (&PreparedChat{}).RuntimeAttachmentRefs() != nil {
		t.Fatal("empty prepared chat returned attachment refs")
	}
}
