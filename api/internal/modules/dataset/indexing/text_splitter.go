package indexing

import (
	"github.com/zgiai/ginext/internal/modules/dataset/splitter"
)

// TextSplitter is an interface for text splitters
type TextSplitter interface {
	SplitText(text string) []string
}

// RecursiveCharacterTextSplitter is a recursive character text splitter implementation
type RecursiveCharacterTextSplitter struct {
	splitter *splitter.RecursiveCharacterTextSplitter
}

// NewRecursiveCharacterTextSplitter creates a new recursive character text splitter
func NewRecursiveCharacterTextSplitter(chunkSize, chunkOverlap int, separators []string) *RecursiveCharacterTextSplitter {
	baseSplitter := splitter.NewRecursiveCharacterTextSplitter(
		separators,
		chunkSize,
		chunkOverlap,
		nil,   // use default length function
		false, // do not keep separator
		false, // do not add start index
	)

	return &RecursiveCharacterTextSplitter{
		splitter: baseSplitter,
	}
}

// SplitText splits text into chunks
func (r *RecursiveCharacterTextSplitter) SplitText(text string) []string {
	return r.splitter.SplitText(text)
}

// FromEncoder creates a recursive character text splitter from an encoder
func (r *RecursiveCharacterTextSplitter) FromEncoder(
	embeddingModelInstance interface{}, // TODO: Replace with actual model instance type
	chunkSize int,
	chunkOverlap int,
	fixedSeparator string,
	separators []string,
	keepSeparator bool,
) *RecursiveCharacterTextSplitter {
	// Use fixed text splitter
	fixedSplitter := splitter.NewFixedRecursiveCharacterTextSplitter(
		fixedSeparator,
		separators,
		chunkSize,
		chunkOverlap,
		nil, // use default length function
		keepSeparator,
		false, // do not add start index
	)

	// Here we simplify the implementation, actually we should use fixedSplitter.FromEncoder method
	// But since we don't have a complete model instance implementation, we use the fixed splitter temporarily

	return &RecursiveCharacterTextSplitter{
		splitter: fixedSplitter.RecursiveCharacterTextSplitter,
	}
}
