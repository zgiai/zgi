package code

import (
	"github.com/zgiai/ginext/internal/modules/app/workflow/file"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
)

func convertEntityFileToWorkflowFile(entityFile *entities.File) *file.File {
	if entityFile == nil {
		return nil
	}

	// Convert transfer method
	var transferMethod file.FileTransferMethod
	switch entityFile.TransferMethod {
	case "local_file":
		transferMethod = file.FileTransferMethodLocalFile
	case "remote_url":
		transferMethod = file.FileTransferMethodRemoteURL
	case "tool_file":
		transferMethod = file.FileTransferMethodToolFile
	default:
		transferMethod = file.FileTransferMethodRemoteURL
	}

	// Convert file type
	var kind file.FileType
	switch entityFile.Type {
	case "image":
		kind = file.FileTypeImage
	case "document":
		kind = file.FileTypeDocument
	case "audio":
		kind = file.FileTypeAudio
	case "video":
		kind = file.FileTypeVideo
	default:
		kind = file.FileTypeCustom
	}

	opts := []file.FileOption{
		file.WithID(entityFile.ID),
		file.WithRelatedID(entityFile.ID),
		file.WithFilename(entityFile.Filename),
		file.WithExtension(entityFile.Extension),
		file.WithMimeType(entityFile.MimeType),
		file.WithSize(int(entityFile.Size)),
	}
	if transferMethod == file.FileTransferMethodRemoteURL && entityFile.RemoteURL != "" {
		opts = append(opts, file.WithRemoteURL(entityFile.RemoteURL))
	}
	if entityFile.RemoteURL != "" {
		opts = append(opts, file.WithURL(entityFile.RemoteURL))
	}

	return file.NewFile(entityFile.WorkspaceID, kind, transferMethod, opts...)
}
