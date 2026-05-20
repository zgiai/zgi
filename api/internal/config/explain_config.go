package config

// PromptTemplates stores all prompt templates for explanation
type PromptTemplates struct {
	Grammar     string `mapstructure:"grammar_template"`
	Vocabulary  string `mapstructure:"vocabulary_template"`
	Translation string `mapstructure:"translation_template"`
	TagExtract  string `mapstructure:"tag_extract_template"`
}

// DefaultPromptTemplates defines default prompt templates
var DefaultPromptTemplates = PromptTemplates{
	Grammar: `You are an English teaching expert. Please provide a detailed grammatical structure analysis of the following English sentence:

Sentence:
"{{.InputText}}"

Requirements:
- Identify the tense, voice, and grammatical structure used in the sentence
- Explain the grammatical function of each component (subject, predicate, object, etc.)
- Point out and explain any difficult or error-prone points if any
- 用中文输出解释，适合英语初学者理解`,

	Vocabulary: `Please extract 3-5 core vocabulary words from the following English sentence and output in the following format:

- Word:
- Part of speech (English):
- Chinese meaning:
- English example sentence (showing usage):

Sentence:
"{{.InputText}}"`,

	Translation: `Please translate the following English sentence into Chinese, output in two styles:

1. Standard Chinese translation: faithful to the original structure
2. Colloquial Chinese translation: more natural and idiomatic expression

Sentence:
"{{.InputText}}"`,

	TagExtract: `Extract 3-5 representative tags from the following English sentence and analysis results, requiring brevity (1-3 words).
Format: One tag per line, no numbering or other symbols. Return results should only include the tag list, no other explanations.

英文Sentence:"{{.InputText}}"

Analysis mode: {{.Mode}}

Analysis result:
{{.ResultContent}}`,
}
