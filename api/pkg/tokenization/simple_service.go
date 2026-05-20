package tokenization

import (
	"regexp"
	"strings"
	"unicode"
)


type SimpleTokenizationService struct{}

func NewSimpleTokenizationService() *SimpleTokenizationService {
	return &SimpleTokenizationService{}
}

func (s *SimpleTokenizationService) Tokenize(text string) ([]string, error) {
	if s.IsChinese(text) {
		return s.tokenizeChinese(text), nil
	}
	return s.tokenizeEnglish(text), nil
}

func (s *SimpleTokenizationService) TokenizeBatch(texts []string) ([][]string, error) {
	results := make([][]string, len(texts))
	for i, t := range texts {
		res, err := s.Tokenize(t)
		if err != nil {
			return nil, err
		}
		results[i] = res
	}
	return results, nil
}

func (s *SimpleTokenizationService) ExtractKeywords(text string, topK int) ([]string, error) {
	tokens, err := s.Tokenize(text)
	if err != nil {
		return nil, err
	}

	keywords := s.filterStopWords(tokens)

	sortByLength(keywords)

	if len(keywords) > topK {
		keywords = keywords[:topK]
	}

	return keywords, nil
}

func (s *SimpleTokenizationService) GetLanguage(text string) string {
	if s.IsChinese(text) {
		return "zh"
	}
	return "en"
}

func (s *SimpleTokenizationService) IsChinese(text string) bool {
	chineseCount := 0
	totalCount := 0

	for _, r := range text {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			continue
		}
		totalCount++
		if unicode.Is(unicode.Han, r) {
			chineseCount++
		}
	}

	if totalCount > 0 && float64(chineseCount)/float64(totalCount) > 0.3 {
		return true
	}
	return false
}

func (s *SimpleTokenizationService) tokenizeChinese(text string) []string {
	var tokens []string
	var currentToken strings.Builder

	for _, r := range text {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			if currentToken.Len() > 0 {
				tokens = append(tokens, currentToken.String())
				currentToken.Reset()
			}
		} else if unicode.Is(unicode.Han, r) {
			if currentToken.Len() > 0 {
				tokens = append(tokens, currentToken.String())
				currentToken.Reset()
			}
			tokens = append(tokens, string(r))
		} else {
			currentToken.WriteRune(r)
		}
	}

	if currentToken.Len() > 0 {
		tokens = append(tokens, currentToken.String())
	}

	return tokens
}

func (s *SimpleTokenizationService) tokenizeEnglish(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	var result []string
	for _, word := range words {
		cleanWord := regexp.MustCompile(`[^\w]`).ReplaceAllString(word, "")
		if len(cleanWord) > 0 {
			result = append(result, cleanWord)
		}
	}
	return result
}

func (s *SimpleTokenizationService) filterStopWords(tokens []string) []string {
	stopWords := map[string]bool{
		"的": true, "了": true, "在": true, "是": true, "我": true, "有": true, "和": true, "就": true, "不": true, "人": true,
		"都": true, "一": true, "一个": true, "上": true, "也": true, "很": true, "到": true, "说": true, "要": true, "去": true,
		"你": true, "会": true, "着": true, "没有": true, "看": true, "好": true, "自己": true, "这": true, "那": true, "他": true,
		"她": true, "它": true, "们": true, "这个": true, "那个": true, "什么": true, "怎么": true, "为什么": true, "哪里": true, "时候": true,

		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "is": true, "are": true, "was": true, "were": true, "be": true, "been": true,
		"have": true, "has": true, "had": true, "do": true, "does": true, "did": true, "will": true, "would": true, "could": true, "should": true,
		"can": true, "may": true, "might": true, "must": true, "shall": true, "this": true, "that": true, "these": true, "those": true,
		"i": true, "you": true, "he": true, "she": true, "it": true, "we": true, "they": true, "me": true, "him": true, "her": true,
		"us": true, "them": true, "my": true, "your": true, "his": true, "its": true, "our": true, "their": true,
	}

	var result []string
	for _, token := range tokens {
		if !stopWords[token] && len(token) > 1 {
			result = append(result, token)
		}
	}
	return result
}

func sortByLength(tokens []string) {
	for i := 0; i < len(tokens)-1; i++ {
		for j := i + 1; j < len(tokens); j++ {
			if len(tokens[i]) < len(tokens[j]) {
				tokens[i], tokens[j] = tokens[j], tokens[i]
			}
		}
	}
}
