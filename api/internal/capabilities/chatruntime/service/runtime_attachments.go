package service

// RuntimeAttachmentRef identifies a file already authorized by chat preparation.
// It contains no file content and is only for caller-scoped runtime staging.
type RuntimeAttachmentRef struct {
	ID   string
	Name string
	Size int64
}

// RuntimeAttachmentRefs returns this prepared turn's authorized attachments.
func (prepared *PreparedChat) RuntimeAttachmentRefs() []RuntimeAttachmentRef {
	if prepared == nil || prepared.parts == nil || prepared.parts.Attachments == nil {
		return nil
	}
	refs := make([]RuntimeAttachmentRef, 0, len(prepared.parts.Attachments.Files))
	for _, file := range prepared.parts.Attachments.Files {
		refs = append(refs, RuntimeAttachmentRef{ID: file.ID, Name: file.Name, Size: file.Size})
	}
	return refs
}
