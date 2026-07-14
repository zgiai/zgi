export type QuestionTypeKey = 'core' | 'extension' | 'fuzzy' | 'manual';

export type QuestionTypeTranslator = (key: QuestionTypeKey) => string;

export const QUESTION_TYPE_OPTIONS: Array<{ value: QuestionTypeKey; labelKey: QuestionTypeKey }> = [
  { value: 'core', labelKey: 'core' },
  { value: 'extension', labelKey: 'extension' },
  { value: 'fuzzy', labelKey: 'fuzzy' },
  { value: 'manual', labelKey: 'manual' },
];

export const DEFAULT_QUESTION_TYPES: QuestionTypeKey[] = ['core'];
export const DEFAULT_TASK_QUESTION_TYPES: QuestionTypeKey[] = ['core'];

export function formatQuestionTypeLabel(
  value: string | null | undefined,
  typeT: QuestionTypeTranslator
): string {
  if (value === 'extension') return typeT('extension');
  if (value === 'fuzzy') return typeT('fuzzy');
  if (value === 'manual') return typeT('manual');
  return typeT('core');
}
