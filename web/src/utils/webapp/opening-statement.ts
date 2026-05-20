export type OpeningStatementType = 'slogan' | 'message';

interface OpeningStatementLike {
  opening_statement_type?: OpeningStatementType | null;
  opening_slogan?: string | null;
  opening_statement?: string | null;
  opening_statement_enabled?: boolean | null;
}

export interface OpeningGuideConfig {
  type: OpeningStatementType;
  slogan?: string;
  message?: string;
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

  const enabled =
    typeof features.opening_statement_enabled === 'boolean'
      ? features.opening_statement_enabled
      : true;

  if (!enabled) {
    return undefined;
  }

  const type = resolveOpeningStatementType(features);
  const slogan =
    typeof features.opening_slogan === 'string'
      ? clampOpeningSlogan(features.opening_slogan)
      : '';
  const message =
    typeof features.opening_statement === 'string' ? features.opening_statement : '';

  if (type === 'slogan') {
    return slogan.trim()
      ? {
          type,
          slogan,
        }
      : undefined;
  }

  return message.trim()
    ? {
        type,
        message,
      }
    : undefined;
}

/**
 * @util Resolve the effective opening statement content from workflow/webapp features.
 * Falls back to enabling legacy content-only data when the explicit flag is missing.
 */
export function getEnabledOpeningStatement(
  features?: OpeningStatementLike | null
): string | undefined {
  const guide = getOpeningGuide(features);
  return guide?.type === 'message' ? guide.message : undefined;
}
