export type OpeningStatementType = 'slogan' | 'message';
export type OpeningGuideMode = 'legacy' | 'combined';

interface OpeningStatementLike {
  opening_statement_type?: OpeningStatementType | null;
  opening_guide_version?: 2 | null;
  opening_slogan?: string | null;
  opening_statement?: string | null;
  opening_statement_enabled?: boolean | null;
  suggested_questions?: string[] | null;
}

export interface OpeningGuideConfig {
  mode: OpeningGuideMode;
  legacyType?: OpeningStatementType;
  title?: string;
  message?: string;
}

export interface OpeningGuideEditorValue {
  title: string;
  message: string;
}

export const OPENING_SLOGAN_MAX_LENGTH = 150;

function clampByCodePoints(value: string, maxLength: number): string {
  return Array.from(value).slice(0, maxLength).join('');
}

export function clampOpeningSlogan(value: string): string {
  return clampByCodePoints(value, OPENING_SLOGAN_MAX_LENGTH);
}

export function resolveOpeningStatementType(
  features?: OpeningStatementLike | null
): OpeningStatementType {
  if (features?.opening_statement_type === 'message') {
    return 'message';
  }

  if (features?.opening_statement_type === 'slogan') {
    return 'slogan';
  }

  const slogan =
    typeof features?.opening_slogan === 'string' ? features.opening_slogan.trim() : '';
  if (slogan) {
    return 'slogan';
  }

  const message =
    typeof features?.opening_statement === 'string' ? features.opening_statement.trim() : '';
  if (message) {
    return 'message';
  }

  return 'slogan';
}

export function getOpeningGuide(
  features?: OpeningStatementLike | null
): OpeningGuideConfig | undefined {
  if (!features) return undefined;

  const slogan =
    typeof features.opening_slogan === 'string'
      ? clampOpeningSlogan(features.opening_slogan)
      : '';
  const message =
    typeof features.opening_statement === 'string' ? features.opening_statement : '';
  const hasSuggestedQuestions =
    Array.isArray(features.suggested_questions) &&
    features.suggested_questions.some(question => question.trim().length > 0);

  if (features.opening_guide_version === 2) {
    return slogan.trim() || message.trim() || hasSuggestedQuestions
      ? {
          mode: 'combined',
          title: slogan.trim() ? slogan : undefined,
          message: message.trim() ? message : undefined,
        }
      : undefined;
  }

  const enabled =
    typeof features.opening_statement_enabled === 'boolean'
      ? features.opening_statement_enabled
      : true;

  if (!enabled) {
    return undefined;
  }

  const type = resolveOpeningStatementType(features);

  if (type === 'slogan') {
    return slogan.trim()
      ? {
          mode: 'legacy',
          legacyType: type,
          title: slogan,
        }
      : undefined;
  }

  return message.trim()
    ? {
        mode: 'legacy',
        legacyType: type,
        message,
      }
    : undefined;
}

export function getOpeningGuideEditorValue(
  features?: OpeningStatementLike | null
): OpeningGuideEditorValue {
  if (!features) {
    return {
      title: '',
      message: '',
    };
  }

  if (features.opening_guide_version === 2) {
    return {
      title:
        typeof features.opening_slogan === 'string'
          ? clampOpeningSlogan(features.opening_slogan)
          : '',
      message:
        typeof features.opening_statement === 'string' ? features.opening_statement : '',
    };
  }

  const type = resolveOpeningStatementType(features);

  return {
    title:
      type === 'slogan' && typeof features.opening_slogan === 'string'
        ? clampOpeningSlogan(features.opening_slogan)
        : '',
    message:
      type === 'message' && typeof features.opening_statement === 'string'
        ? features.opening_statement
        : '',
  };
}

/**
 * @util Resolve the effective opening statement content from workflow/webapp features.
 * Falls back to enabling legacy content-only data when the explicit flag is missing.
 */
export function getEnabledOpeningStatement(
  features?: OpeningStatementLike | null
): string | undefined {
  const guide = getOpeningGuide(features);
  return guide?.message;
}
