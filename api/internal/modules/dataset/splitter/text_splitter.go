package splitter

import (
	"strings"
)

// TextSplitter interface for text splitters
type TextSplitter interface {
	SplitText(text string) []string
}

// BaseTextSplitter base text splitter structure
type BaseTextSplitter struct {
	ChunkSize      int
	ChunkOverlap   int
	LengthFunction func([]string) []int
	KeepSeparator  bool
	AddStartIndex  bool
}

// NewBaseTextSplitter creates a new base text splitter
func NewBaseTextSplitter(chunkSize, chunkOverlap int, lengthFunction func([]string) []int, keepSeparator, addStartIndex bool) *BaseTextSplitter {
	if chunkOverlap > chunkSize {
		panic("chunkOverlap should be smaller than chunkSize")
	}

	if lengthFunction == nil {
		lengthFunction = func(texts []string) []int {
			lengths := make([]int, len(texts))
			for i, text := range texts {
				// Use rune count (character count) instead of byte length
				// This properly handles multi-byte characters like Chinese
				lengths[i] = len([]rune(text))
			}
			return lengths
		}
	}

	return &BaseTextSplitter{
		ChunkSize:      chunkSize,
		ChunkOverlap:   chunkOverlap,
		LengthFunction: lengthFunction,
		KeepSeparator:  keepSeparator,
		AddStartIndex:  addStartIndex,
	}
}

// CharacterTextSplitter character-based text splitter
type CharacterTextSplitter struct {
	*BaseTextSplitter
	Separator string
}

// NewCharacterTextSplitter creates a new character-based text splitter
func NewCharacterTextSplitter(separator string, chunkSize, chunkOverlap int, lengthFunction func([]string) []int, keepSeparator, addStartIndex bool) *CharacterTextSplitter {
	return &CharacterTextSplitter{
		BaseTextSplitter: NewBaseTextSplitter(chunkSize, chunkOverlap, lengthFunction, keepSeparator, addStartIndex),
		Separator:        separator,
	}
}

// SplitText implements text splitting functionality
func (c *CharacterTextSplitter) SplitText(text string) []string {
	// First split the large text into smaller parts
	splits := splitTextWithRegex(text, c.Separator, c.KeepSeparator)
	separator := ""
	if !c.KeepSeparator {
		separator = c.Separator
	}

	// Remove whitespace
	splits = c.trimWhitespace(splits)

	// Cache split lengths
	goodSplitsLengths := make([]int, 0)
	if len(splits) > 0 {
		goodSplitsLengths = c.LengthFunction(splits)
	}

	return c.mergeSplits(splits, separator, goodSplitsLengths)
}

// trimWhitespace removes whitespace characters from string slice
func (c *CharacterTextSplitter) trimWhitespace(splits []string) []string {
	result := make([]string, 0, len(splits))
	for _, s := range splits {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" && trimmed != "\n" {
			result = append(result, trimmed)
		}
	}
	return result
}

// splitTextWithRegex splits text using regular expressions
func splitTextWithRegex(text, separator string, keepSeparator bool) []string {
	var splits []string

	if separator != "" {
		if keepSeparator {
			parts := strings.Split(text, separator)
			for i, part := range parts {
				if i > 0 {
					splits = append(splits, separator+part)
				} else {
					splits = append(splits, part)
				}
			}
		} else {
			splits = strings.Split(text, separator)
		}
	} else {
		for _, char := range text {
			splits = append(splits, string(char))
		}
	}

	var result []string
	for _, s := range splits {
		if s != "" && s != "\n" {
			result = append(result, s)
		}
	}

	return result
}

// joinDocs joins documents together with a separator
func (b *BaseTextSplitter) joinDocs(docs []string, separator string) *string {
	text := strings.Join(docs, separator)
	text = strings.TrimSpace(text)

	if text == "" {
		return nil
	}

	return &text
}

// mergeSplits merges split text segments according to chunk size and overlap constraints
func (b *BaseTextSplitter) mergeSplits(splits []string, separator string, lengths []int) []string {
	separatorLen := 0
	if len(b.LengthFunction([]string{separator})) > 0 {
		separatorLen = b.LengthFunction([]string{separator})[0]
	}

	docs := make([]string, 0)
	currentDoc := make([]string, 0)
	total := 0
	index := 0

	for _, d := range splits {
		var length int
		if index < len(lengths) {
			length = lengths[index]
		}

		if total+length+(separatorLenIfNotEmpty(currentDoc, separatorLen)) > b.ChunkSize {
			if total > b.ChunkSize {
				// logger.Warning(fmt.Sprintf("Created a chunk of size %d, which is longer than the specified %d", total, b.ChunkSize))
			}

			if len(currentDoc) > 0 {
				doc := b.joinDocs(currentDoc, separator)
				if doc != nil {
					docs = append(docs, *doc)

					for total > b.ChunkOverlap ||
						(total+length+(separatorLenIfNotEmpty(currentDoc, separatorLen)) > b.ChunkSize && total > 0) {
						firstLen := 0
						if len(b.LengthFunction([]string{currentDoc[0]})) > 0 {
							firstLen = b.LengthFunction([]string{currentDoc[0]})[0]
						}

						separatorLenToRemove := 0
						if len(currentDoc) > 1 {
							separatorLenToRemove = separatorLen
						}

						total -= firstLen + separatorLenToRemove
						currentDoc = currentDoc[1:]
					}
				}
			}
		}

		currentDoc = append(currentDoc, d)
		total += length + separatorLenIfNotEmpty(currentDoc[1:], separatorLen)
		index++
	}

	doc := b.joinDocs(currentDoc, separator)
	if doc != nil {
		docs = append(docs, *doc)
	}

	return docs
}

// separatorLenIfNotEmpty returns separator length if document list is not empty
func separatorLenIfNotEmpty(docs []string, separatorLen int) int {
	if len(docs) > 0 {
		return separatorLen
	}
	return 0
}
