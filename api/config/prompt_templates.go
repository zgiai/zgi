package config

// PromptTemplates stores all prompt templates
type PromptTemplates struct {
	Grammar     string `mapstructure:"grammar_template"`
	Vocabulary  string `mapstructure:"vocabulary_template"`
	Translation string `mapstructure:"translation_template"`
	TagExtract  string `mapstructure:"tag_extract_template"`
}

// DefaultPromptTemplates defines default prompt templates
var DefaultPromptTemplates = PromptTemplates{
	Grammar: `你是一个英语教学专家，请对以下英文句子进行详细的语法结构分析：

句子：
"{{.InputText}}"

要求：
- 指出句子使用的时态、语态、语法结构
- 解释每个成分的语法功能（主语、谓语、宾语等）
- 如有难点或易错点，请指出并解释
- 用中文输出解释，适合英语初学者理解`,

	Vocabulary: `请从以下英文句子中提取3~5个核心词汇，输出以下格式：

- 单词：
- 词性（英文）：
- 中文含义：
- 英文例句（展示用法）：

句子：
"{{.InputText}}"`,

	Translation: `请将以下英文句子翻译成中文，输出两种风格：

1. 标准中文翻译：忠实于原文结构
2. 口语化中文翻译：更自然、更地道的表达

句子：
"{{.InputText}}"`,

	TagExtract: `从以下英文句子和分析结果中，提取3-5个有代表性的标签，要求简短（1-3个单词）。
格式：每行一个标签，不要编号或其他符号。返回结果仅包含标签列表，不要其他解释。

英文句子："{{.InputText}}"

分析模式：{{.Mode}}

分析结果：
{{.ResultContent}}`,
}
