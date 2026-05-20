package tokenization


type TokenizationService interface {
	Tokenize(text string) ([]string, error)

	TokenizeBatch(texts []string) ([][]string, error)

	ExtractKeywords(text string, topK int) ([]string, error)

	GetLanguage(text string) string

	IsChinese(text string) bool
}
