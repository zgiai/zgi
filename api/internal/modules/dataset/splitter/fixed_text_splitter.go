package splitter

import (
	"strings"
)

// EnhanceRecursiveCharacterTextSplitter enhances the recursive character text splitter
type EnhanceRecursiveCharacterTextSplitter struct {
	*RecursiveCharacterTextSplitter
}

// NewEnhanceRecursiveCharacterTextSplitter creates a new enhanced recursive character text splitter
func NewEnhanceRecursiveCharacterTextSplitter(separators []string, chunkSize, chunkOverlap int, lengthFunction func([]string) []int, keepSeparator, addStartIndex bool) *EnhanceRecursiveCharacterTextSplitter {
	return &EnhanceRecursiveCharacterTextSplitter{
		RecursiveCharacterTextSplitter: NewRecursiveCharacterTextSplitter(separators, chunkSize, chunkOverlap, lengthFunction, keepSeparator, addStartIndex),
	}
}

// FromEncoder creates an enhanced recursive character text splitter from an encoder
func (e *EnhanceRecursiveCharacterTextSplitter) FromEncoder(
		embeddingModelInstance interface{}, // TODO: replace with the actual model instance type
	allowedSpecial []string,
	disallowedSpecial []string,
	chunkSize int,
	chunkOverlap int,
	separators []string,
	keepSeparator bool,
	addStartIndex bool,
) *EnhanceRecursiveCharacterTextSplitter {
	// Define the token encoder function
	tokenEncoder := func(texts []string) []int {
		if len(texts) == 0 {
			return []int{}
		}

			// TODO: implement actual token encoding logic
			// If an embedding model instance is available, use it to get the token count.
			// Otherwise use the Go version of GPT2Tokenizer.
		lengths := make([]int, len(texts))
		for i, text := range texts {
			// Use rune count (character count) instead of byte length.
			// This handles multi-byte characters such as Chinese correctly.
			lengths[i] = len([]rune(text))
		}
		return lengths
	}

	return NewEnhanceRecursiveCharacterTextSplitter(separators, chunkSize, chunkOverlap, tokenEncoder, keepSeparator, addStartIndex)
}

// FixedRecursiveCharacterTextSplitter fixed recursive character text splitter
type FixedRecursiveCharacterTextSplitter struct {
	*EnhanceRecursiveCharacterTextSplitter
	FixedSeparator string
	Separators     []string
}

// NewFixedRecursiveCharacterTextSplitter creates a new fixed recursive character text splitter
func NewFixedRecursiveCharacterTextSplitter(fixedSeparator string, separators []string, chunkSize, chunkOverlap int, lengthFunction func([]string) []int, keepSeparator, addStartIndex bool) *FixedRecursiveCharacterTextSplitter {
	return &FixedRecursiveCharacterTextSplitter{
		EnhanceRecursiveCharacterTextSplitter: NewEnhanceRecursiveCharacterTextSplitter(separators, chunkSize, chunkOverlap, lengthFunction, keepSeparator, addStartIndex),
		FixedSeparator:                        fixedSeparator,
		Separators:                            separators,
	}
}

// FromEncoder creates a fixed recursive character text splitter from an encoder
func (f *FixedRecursiveCharacterTextSplitter) FromEncoder(
		embeddingModelInstance interface{}, // TODO: replace with the actual model instance type
	fixedSeparator string,
	allowedSpecial []string,
	disallowedSpecial []string,
	chunkSize int,
	chunkOverlap int,
	separators []string,
	keepSeparator bool,
	addStartIndex bool,
) *FixedRecursiveCharacterTextSplitter {
	// Define the token encoder function
	tokenEncoder := func(texts []string) []int {
		if len(texts) == 0 {
			return []int{}
		}

			// TODO: implement actual token encoding logic
			// If an embedding model instance is available, use it to get the token count.
			// Otherwise use the Go version of GPT2Tokenizer.
		lengths := make([]int, len(texts))
		for i, text := range texts {
			// Use rune count (character count) instead of byte length.
			// This handles multi-byte characters such as Chinese correctly.
			lengths[i] = len([]rune(text))
		}
		return lengths
	}

	return NewFixedRecursiveCharacterTextSplitter(fixedSeparator, separators, chunkSize, chunkOverlap, tokenEncoder, keepSeparator, addStartIndex)
}

// SplitText implements text splitting
func (f *FixedRecursiveCharacterTextSplitter) SplitText(text string) []string {
	// Normalize line endings: convert Windows CRLF to Unix LF
	text = strings.ReplaceAll(text, "\r\n", "\n")
	// Also handle standalone CR (old Mac format) just in case
	text = strings.ReplaceAll(text, "\r", "\n")

	var chunks []string
	if f.FixedSeparator != "" {
		chunks = strings.Split(text, f.FixedSeparator)
	} else {
		chunks = []string{text}
	}

	finalChunks := make([]string, 0)
	chunksLengths := f.LengthFunction(chunks)
	
	for i, chunk := range chunks {
		var chunkLength int
		if i < len(chunksLengths) {
			chunkLength = chunksLengths[i]
		}
		
		if chunkLength > f.ChunkSize {
			finalChunks = append(finalChunks, f.recursiveSplitText(chunk)...)
		} else {
			finalChunks = append(finalChunks, chunk)
		}
	}

	return finalChunks
}

// recursiveSplitText recursively splits text
func (f *FixedRecursiveCharacterTextSplitter) recursiveSplitText(text string) []string {
	finalChunks := make([]string, 0)
	
	// Select the appropriate separator
	separator := f.Separators[len(f.Separators)-1]
	for _, s := range f.Separators {
		if s == "" {
			separator = s
			break
		}
		if strings.Contains(text, s) {
			separator = s
			break
		}
	}
	
	// Now that we have the separator, split the text
	var splits []string
	if separator != "" {
		splits = strings.Split(text, separator)
	} else {
		splits = []string{text}
	}
	
	// If splitting produces a single segment identical to the original text, it cannot be split further
	if len(splits) == 1 && splits[0] == text {
		// When no further split is possible, force a character-based split or return the original text.
		// Use rune count (character count) for proper handling of multi-byte characters.
		runes := []rune(text)
		if len(runes) > f.ChunkSize {
			// If the text still exceeds ChunkSize, force a character-based split by ChunkSize.
			var forcedSplits []string
			for i := 0; i < len(runes); i += f.ChunkSize {
				end := i + f.ChunkSize
				if end > len(runes) {
					end = len(runes)
				}
				forcedSplits = append(forcedSplits, string(runes[i:end]))
			}
			return forcedSplits
		}
		return []string{text}
	}
	
	// Now start merging content and recursively split longer text
	goodSplits := make([]string, 0)
	goodSplitsLengths := make([]int, 0)
	sLens := f.LengthFunction(splits)
	
	for i, s := range splits {
		sLen := 0
		if i < len(sLens) {
			sLen = sLens[i]
		}
		
		if sLen < f.ChunkSize {
			goodSplits = append(goodSplits, s)
			goodSplitsLengths = append(goodSplitsLengths, sLen)
		} else {
			if len(goodSplits) > 0 {
				mergedText := f.mergeSplits(goodSplits, separator, goodSplitsLengths)
				finalChunks = append(finalChunks, mergedText...)
				goodSplits = make([]string, 0)
				goodSplitsLengths = make([]int, 0)
			}
			otherInfo := f.recursiveSplitText(s)
			finalChunks = append(finalChunks, otherInfo...)
		}
	}
	
	if len(goodSplits) > 0 {
		mergedText := f.mergeSplits(goodSplits, separator, goodSplitsLengths)
		finalChunks = append(finalChunks, mergedText...)
	}
	
	return finalChunks
}
