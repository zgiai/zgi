/**
 * An AIChat error frame carries two codes: `code`, the product code the
 * frontend already understands (a numeric billing code or a runtime word such
 * as model_service_timeout), and `error_code`, the stable catalog identity
 * (for example llm.model.not_found). An error without a product code arrives
 * with the generic system code 399001, which renders as a server error; for
 * those the catalog identity is the code to show.
 *
 * Kept free of path aliases so the Node test runner can import it directly.
 */
const GENERIC_SYSTEM_ERROR_CODE = '399001';

// Same prefixes as isApplicationErrorCode in services/application-error-i18n.
const APPLICATION_CODE_PREFIXES = ['billing.', 'llm.', 'video.', 'image.', 'music.'];

export function isAIChatApplicationErrorCode(code: unknown): code is string {
  return (
    typeof code === 'string' &&
    APPLICATION_CODE_PREFIXES.some(prefix => code.trim().startsWith(prefix))
  );
}

export function resolveAIChatErrorCode(
  code: string | number | undefined,
  errorCode: string | undefined
): string | number | undefined {
  const productCode = code === undefined || code === null ? '' : String(code).trim();
  const catalogCode = typeof errorCode === 'string' ? errorCode.trim() : '';
  if (
    (productCode === '' || productCode === GENERIC_SYSTEM_ERROR_CODE) &&
    isAIChatApplicationErrorCode(catalogCode)
  ) {
    return catalogCode;
  }
  return code;
}
