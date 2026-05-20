package model

import "errors"

// File-related errors
var (
	ErrNoFileUploaded         = errors.New("no file uploaded")
	ErrTooManyFiles           = errors.New("too many files")
	ErrFilenameNotExists      = errors.New("filename not exists")
	ErrFileTooLarge           = errors.New("file too large")
	ErrUnsupportedFileType    = errors.New("unsupported file type")
	ErrFileNotFound           = errors.New("file not found")
	ErrInvalidFileSignature   = errors.New("invalid file signature")
	ErrFileProcessingFailed   = errors.New("file processing failed")
	ErrInsufficientPermission = errors.New("insufficient permission for datasets")
)

// FileError File error structure
type FileError struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Description string `json:"description,omitempty"`
}

func (e *FileError) Error() string {
	return e.Message
}

// NewFileError Create a file error
func NewFileError(code, message, description string) *FileError {
	return &FileError{
		Code:        code,
		Message:     message,
		Description: description,
	}
}

// Predefined file errors
var (
	FileErrorNoFileUploaded = &FileError{
		Code:    "no_file_uploaded",
		Message: "Please upload your file.",
	}

	FileErrorTooManyFiles = &FileError{
		Code:    "too_many_files",
		Message: "Only one file is allowed.",
	}

	FileErrorFilenameNotExists = &FileError{
		Code:    "filename_not_exists",
		Message: "Please upload a file with a filename.",
	}

	FileErrorFileTooLarge = &FileError{
		Code:    "file_too_large",
		Message: "File size exceeds the limit.",
	}

	FileErrorUnsupportedFileType = &FileError{
		Code:    "unsupported_file_type",
		Message: "File type not allowed.",
	}

	FileErrorFileNotFound = &FileError{
		Code:    "file_not_found",
		Message: "File not found.",
	}

	FileErrorInsufficientPermission = &FileError{
		Code:    "insufficient_permission",
		Message: "Insufficient permission for datasets.",
	}
)
