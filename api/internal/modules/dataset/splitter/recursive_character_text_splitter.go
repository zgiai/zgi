package splitter

import "strings"

// RecursiveCharacterTextSplitter recursive character text splitter
type RecursiveCharacterTextSplitter struct {
	*BaseTextSplitter
	Separators []string
}

// NewRecursiveCharacterTextSplitter creates a new recursive character text splitter
func NewRecursiveCharacterTextSplitter(separators []string, chunkSize, chunkOverlap int, lengthFunction func([]string) []int, keepSeparator, addStartIndex bool) *RecursiveCharacterTextSplitter {
	// If no separators are provided, use default separators
	if separators == nil {
		separators = []string{"\n\n", "\n", " ", ""}
	}

	return &RecursiveCharacterTextSplitter{
		BaseTextSplitter: NewBaseTextSplitter(chunkSize, chunkOverlap, lengthFunction, keepSeparator, addStartIndex),
		Separators:       separators,
	}
}

// SplitText implements text splitting
func (r *RecursiveCharacterTextSplitter) SplitText(text string) []string {
	return r.splitText(text, r.Separators)
}

// splitText recursively splits text
func (r *RecursiveCharacterTextSplitter) splitText(text string, separators []string) []string {
	finalChunks := make([]string, 0)
	separator := separators[len(separators)-1]
	newSeparators := make([]string, 0)

	for i, s := range separators {
		if s == "" {
			separator = s
			break
		}
		if stringsContains(text, s) {
			separator = s
			newSeparators = separators[i+1:]
			break
		}
	}

	splits := splitTextWithRegex(text, separator, r.KeepSeparator)

	// If only one fragment is split and it is the same as the original text, it means it cannot be split further, return directly
	if len(splits) == 1 && splits[0] == text && len(newSeparators) == 0 {
		// When further splitting is not possible, force character splitting or return original text
		// Use rune count (character count) for proper handling of multi-byte characters
		runes := []rune(text)
		if len(runes) > r.ChunkSize {
			// If text length still exceeds ChunkSize, force split by ChunkSize (character-based)
			var forcedSplits []string
			for i := 0; i < len(runes); i += r.ChunkSize {
				end := i + r.ChunkSize
				if end > len(runes) {
					end = len(runes)
				}
				forcedSplits = append(forcedSplits, string(runes[i:end]))
			}
			return forcedSplits
		}
		return []string{text}
	}

	goodSplits := make([]string, 0)
	goodSplitsLengths := make([]int, 0)
	sep := ""
	if !r.KeepSeparator {
		sep = separator
	}

	sLens := r.LengthFunction(splits)
	for i, s := range splits {
		sLen := 0
		if i < len(sLens) {
			sLen = sLens[i]
		}

		if sLen < r.ChunkSize {
			goodSplits = append(goodSplits, s)
			goodSplitsLengths = append(goodSplitsLengths, sLen)
		} else {
			if len(goodSplits) > 0 {
				mergedText := r.mergeSplits(goodSplits, sep, goodSplitsLengths)
				finalChunks = append(finalChunks, mergedText...)
				goodSplits = make([]string, 0)
				goodSplitsLengths = make([]int, 0)
			}
			if len(newSeparators) == 0 {
				// When no more separators are available, if the text is still too large, force splitting
				// Use rune count (character count) for proper handling of multi-byte characters
				runes := []rune(s)
				if len(runes) > r.ChunkSize {
					for i := 0; i < len(runes); i += r.ChunkSize {
						end := i + r.ChunkSize
						if end > len(runes) {
							end = len(runes)
						}
						finalChunks = append(finalChunks, string(runes[i:end]))
					}
				} else {
					finalChunks = append(finalChunks, s)
				}
			} else {
				otherInfo := r.splitText(s, newSeparators)
				finalChunks = append(finalChunks, otherInfo...)
			}
		}
	}

	if len(goodSplits) > 0 {
		mergedText := r.mergeSplits(goodSplits, sep, goodSplitsLengths)
		finalChunks = append(finalChunks, mergedText...)
	}

	return finalChunks
}

// stringsContains checks if a string contains a substring
func stringsContains(text, substr string) bool {
	return strings.Contains(text, substr)
}

// FromEncoder creates a recursive character text splitter from an encoder
func (r *RecursiveCharacterTextSplitter) FromEncoder(
	embeddingModelInstance interface{}, // TODO: use actual model instance type
	allowedSpecial []string,
	disallowedSpecial []string,
	chunkSize int,
	chunkOverlap int,
	separators []string,
	keepSeparator bool,
	addStartIndex bool,
) *RecursiveCharacterTextSplitter {
	// Define token encoder function
	tokenEncoder := func(texts []string) []int {
		if len(texts) == 0 {
			return []int{}
		}

		// TODO: Implement actual token encoding logic
		lengths := make([]int, len(texts))
		for i, text := range texts {
			// Use rune count (character count) instead of byte length
			// This properly handles multi-byte characters like Chinese
			lengths[i] = len([]rune(text))
		}
		return lengths
	}

	return NewRecursiveCharacterTextSplitter(separators, chunkSize, chunkOverlap, tokenEncoder, keepSeparator, addStartIndex)
}
