export type QuestionTypeTranslator = (key: 'core' | 'extension' | 'fuzzy') => string;

export function formatQuestionTypeLabel(
  value: string | null | undefined,
  typeT: QuestionTypeTranslator
): string {
  if (value === 'extension') return typeT('extension');
  if (value === 'fuzzy') return typeT('fuzzy');
  return typeT('core');
}
