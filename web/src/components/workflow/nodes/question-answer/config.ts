import type { ValidationError, ValidationResult } from '../common/validation';

export type QuestionAnswerType = 'text' | 'choice';
export type QuestionAnswerChoiceMode = 'static' | 'dynamic';
export type QuestionAnswerExtractionFieldType = 'string' | 'number' | 'boolean';

export interface QuestionAnswerModelConfig {
  provider: string;
  name?: string;
  model?: string;
  completion_params?: Record<string, string | number | boolean>;
}

export interface QuestionAnswerChoice {
  id: string;
  label: string;
  value: string;
}

export interface QuestionAnswerExtractionField {
  name: string;
  type: QuestionAnswerExtractionFieldType;
  required: boolean;
  description: string;
}

export interface QuestionAnswerNodeData {
  type: 'question-answer';
  title: string;
  desc: string;
  question: string;
  answer_type: QuestionAnswerType;
  completion_instruction: string;
  extract_from_answer: boolean;
  extraction_instruction: string;
  extraction_fields: QuestionAnswerExtractionField[];
  max_answer_count: number;
  model: QuestionAnswerModelConfig;
  model_config: QuestionAnswerModelConfig;
  choices: QuestionAnswerChoice[];
  dynamic_choices: {
    selector: string[];
  };
  choice_mode: QuestionAnswerChoiceMode;
  isInLoop?: boolean;
  isInIteration?: boolean;
}

export const QUESTION_ANSWER_DYNAMIC_HANDLE = 'dynamicOption';
export const QUESTION_ANSWER_MAX_ANSWER_COUNT = 10;

export const DEFAULT_QUESTION_ANSWER_NODE_DATA: QuestionAnswerNodeData = {
  type: 'question-answer',
  title: 'Question Answer',
  desc: '',
  question: '',
  answer_type: 'text',
  completion_instruction: '',
  extract_from_answer: false,
  extraction_instruction: '',
  extraction_fields: [],
  max_answer_count: 3,
  model: {
    provider: '',
    name: '',
    completion_params: {},
  },
  model_config: {
    provider: '',
    name: '',
    completion_params: {},
  },
  choices: [
    { id: 'A', label: '', value: 'A' },
    { id: 'B', label: '', value: 'B' },
  ],
  dynamic_choices: {
    selector: [],
  },
  choice_mode: 'static',
  isInLoop: false,
  isInIteration: false,
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function normalizeString(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

function normalizeModel(value: unknown): QuestionAnswerModelConfig {
  const record = isRecord(value) ? value : {};
  const completionParams = isRecord(record.completion_params) ? record.completion_params : {};
  const params: Record<string, string | number | boolean> = {};
  Object.entries(completionParams).forEach(([key, param]) => {
    if (typeof param === 'string' || typeof param === 'number' || typeof param === 'boolean') {
      params[key] = param;
    }
  });

  return {
    provider: normalizeString(record.provider),
    name: normalizeString(record.name),
    model: normalizeString(record.model),
    completion_params: params,
  };
}

function normalizeChoice(value: unknown, index: number): QuestionAnswerChoice {
  const record = isRecord(value) ? value : {};
  const fallbackID = String.fromCharCode(65 + index);
  const id = normalizeString(record.id).trim() || fallbackID;
  const label = normalizeString(record.label).trim();
  const rawValue = normalizeString(record.value).trim();

  return {
    id,
    label,
    value: rawValue || id,
  };
}

function isExtractionFieldType(value: unknown): value is QuestionAnswerExtractionFieldType {
  return value === 'string' || value === 'number' || value === 'boolean';
}

function normalizeExtractionField(value: unknown, index: number): QuestionAnswerExtractionField {
  const record = isRecord(value) ? value : {};
  void index;
  return {
    name: normalizeString(record.name).trim(),
    type: isExtractionFieldType(record.type) ? record.type : 'string',
    required: typeof record.required === 'boolean' ? record.required : true,
    description: normalizeString(record.description),
  };
}

function normalizeExtractionFields(value: unknown): QuestionAnswerExtractionField[] {
  if (!Array.isArray(value)) return [];
  return value.map(normalizeExtractionField);
}

function normalizeChoices(value: unknown): QuestionAnswerChoice[] {
  if (!Array.isArray(value)) return DEFAULT_QUESTION_ANSWER_NODE_DATA.choices;
  return value.map(normalizeChoice).filter(choice => choice.id || choice.label || choice.value);
}

function normalizeSelector(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : [];
}

export function normalizeQuestionAnswerNodeData(
  value: Partial<QuestionAnswerNodeData> | Record<string, unknown> | null | undefined
): QuestionAnswerNodeData {
  const record = isRecord(value) ? value : {};
  const answerType = record.answer_type === 'choice' ? 'choice' : 'text';
  const dynamicChoices = isRecord(record.dynamic_choices) ? record.dynamic_choices : {};
  const model = normalizeModel(record.model);
  const modelConfig = normalizeModel(record.model_config);
  const choiceMode =
    record.choice_mode === 'dynamic' || normalizeSelector(dynamicChoices.selector).length > 0
      ? 'dynamic'
      : 'static';

  return {
    ...DEFAULT_QUESTION_ANSWER_NODE_DATA,
    ...record,
    type: 'question-answer',
    title:
      normalizeString(record.title).trim() || DEFAULT_QUESTION_ANSWER_NODE_DATA.title,
    desc: normalizeString(record.desc),
    question: normalizeString(record.question),
    answer_type: answerType,
    completion_instruction: normalizeString(record.completion_instruction),
    extract_from_answer: Boolean(record.extract_from_answer),
    extraction_instruction:
      normalizeString(record.extraction_instruction) || normalizeString(record.completion_instruction),
    extraction_fields: normalizeExtractionFields(record.extraction_fields),
    max_answer_count:
      typeof record.max_answer_count === 'number' && Number.isFinite(record.max_answer_count)
        ? Math.max(1, Math.min(QUESTION_ANSWER_MAX_ANSWER_COUNT, Math.floor(record.max_answer_count)))
        : DEFAULT_QUESTION_ANSWER_NODE_DATA.max_answer_count,
    model: model.provider || model.name || model.model ? model : modelConfig,
    model_config: modelConfig.provider || modelConfig.name || modelConfig.model ? modelConfig : model,
    choices: normalizeChoices(record.choices),
    dynamic_choices: {
      selector: normalizeSelector(dynamicChoices.selector),
    },
    choice_mode: choiceMode,
    isInLoop: Boolean(record.isInLoop),
    isInIteration: Boolean(record.isInIteration),
  };
}

export function createQuestionAnswerChoice(
  existingChoices: QuestionAnswerChoice[]
): QuestionAnswerChoice {
  const existing = new Set(existingChoices.map(choice => choice.id));
  let index = existingChoices.length;
  let id = String.fromCharCode(65 + index);
  while (existing.has(id)) {
    index += 1;
    id = `option_${index + 1}`;
  }
  return { id, label: '', value: id };
}

export function createQuestionAnswerExtractionField(
  existingFields: QuestionAnswerExtractionField[]
): QuestionAnswerExtractionField {
  const existing = new Set(existingFields.map(field => field.name));
  let index = existingFields.length + 1;
  let name = `field_${index}`;
  while (existing.has(name)) {
    index += 1;
    name = `field_${index}`;
  }
  return { name, type: 'string', required: true, description: '' };
}

export function getQuestionAnswerOutputVariables(data: QuestionAnswerNodeData) {
  const base = [
    { key: 'question', type: 'string' as const },
    { key: 'answer', type: 'string' as const },
    { key: 'answers', type: 'array[object]' as const },
    { key: 'round', type: 'number' as const },
    { key: 'complete', type: 'boolean' as const },
  ];

  if (data.answer_type !== 'choice') {
    if (!data.extract_from_answer) return base;
    return [
      ...base,
      { key: 'extracted_fields', type: 'object' as const },
      ...data.extraction_fields
        .filter(field => field.name.trim())
        .map(field => ({ key: field.name.trim(), type: field.type })),
    ];
  }

  return [
    ...base,
    { key: 'choices', type: 'array[object]' as const },
    { key: 'choice_id', type: 'string' as const },
    { key: 'choice_label', type: 'string' as const },
    { key: 'choice_value', type: 'string' as const },
  ];
}

export function checkValid(data: QuestionAnswerNodeData): ValidationResult {
  const normalized = normalizeQuestionAnswerNodeData(data);
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  if (!normalized.question.trim()) {
    errors.push({ code: 'questionAnswer.validation.questionRequired' });
  }

  if (normalized.answer_type === 'text' && normalized.extract_from_answer) {
    const modelName = normalized.model.name || normalized.model.model || '';
    if (!normalized.model.provider || !modelName) {
      errors.push({ code: 'questionAnswer.validation.modelRequired' });
    }
    if (normalized.extraction_fields.length === 0) {
      errors.push({ code: 'questionAnswer.validation.extractionFieldsRequired' });
    }
    const seen = new Set<string>();
    normalized.extraction_fields.forEach((field, index) => {
      const name = field.name.trim();
      if (!name) {
        errors.push({
          code: 'questionAnswer.validation.extractionFieldNameRequired',
          params: { index: index + 1 },
        });
      } else if (seen.has(name)) {
        errors.push({
          code: 'questionAnswer.validation.extractionFieldNameDuplicate',
          params: { name },
        });
      }
      seen.add(name);
    });
  }

  if (normalized.answer_type === 'choice') {
    if (normalized.choice_mode === 'dynamic') {
      if (normalized.dynamic_choices.selector.length < 2) {
        errors.push({ code: 'questionAnswer.validation.dynamicSelectorRequired' });
      }
    } else {
      const choices = normalized.choices;
      if (choices.length === 0) {
        errors.push({ code: 'questionAnswer.validation.choicesRequired' });
      }
      const seen = new Set<string>();
      choices.forEach((choice, index) => {
        const id = choice.id.trim();
        if (!id) {
          errors.push({
            code: 'questionAnswer.validation.choiceIdRequired',
            params: { index: index + 1 },
          });
        } else if (seen.has(id)) {
          errors.push({
            code: 'questionAnswer.validation.choiceIdDuplicate',
            params: { id },
          });
        }
        seen.add(id);
      });
    }
  }

  return { isValid: errors.length === 0, errors, warnings };
}
