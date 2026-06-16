/**
 * Validation utility functions
 * Common validation operations for different data types
 */

/**
 * Validates an email address
 * @param email - Email to validate
 */
export function isValidEmail(email: string): boolean {
  const pattern = /^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/;
  return pattern.test(email);
}

/**
 * Validates a password against common security rules
 * @param password - Password to validate
 * @param options - Validation options
 */
export function isValidPassword(
  password: string,
  options = {
    minLength: 8,
    requireNumbers: true,
    requireSpecialChars: true,
    requireUppercase: true,
    requireLowercase: true,
  }
): { valid: boolean; errors: string[] } {
  const errors: string[] = [];

  if (password.length < options.minLength) {
    errors.push(`Password must be at least ${options.minLength} characters long`);
  }

  if (options.requireNumbers && !/\d/.test(password)) {
    errors.push('Password must include at least one number');
  }

  if (options.requireSpecialChars && !/[!@#$%^&*(),.?":{}|<>]/.test(password)) {
    errors.push('Password must include at least one special character');
  }

  if (options.requireUppercase && !/[A-Z]/.test(password)) {
    errors.push('Password must include at least one uppercase letter');
  }

  if (options.requireLowercase && !/[a-z]/.test(password)) {
    errors.push('Password must include at least one lowercase letter');
  }

  return {
    valid: errors.length === 0,
    errors,
  };
}

/**
 * Validates a URL
 * @param url - URL to validate
 */
export function isValidUrl(url: string): boolean {
  try {
    new URL(url);
    return true;
  } catch {
    return false;
  }
}

/**
 * Validates a credit card number using the Luhn algorithm
 * @param cardNumber - Credit card number to validate
 */
export function isValidCreditCard(cardNumber: string): boolean {
  // Remove spaces and non-digit characters
  const digits = cardNumber.replace(/\D/g, '');

  if (digits.length < 13 || digits.length > 19) {
    return false;
  }

  // Luhn algorithm
  let sum = 0;
  let alternate = false;

  for (let i = digits.length - 1; i >= 0; i--) {
    let digit = parseInt(digits.charAt(i), 10);

    if (alternate) {
      digit *= 2;
      if (digit > 9) {
        digit -= 9;
      }
    }

    sum += digit;
    alternate = !alternate;
  }

  return sum % 10 === 0;
}

/**
 * Validates a phone number
 * @param phoneNumber - Phone number to validate
 * @param countryCode - Country code (default: 'US')
 */
export function isValidPhoneNumber(phoneNumber: string, countryCode = 'US'): boolean {
  // Remove non-digit characters
  const digits = phoneNumber.replace(/\D/g, '');

  if (countryCode === 'US') {
    // US phone number validation (10 digits)
    return digits.length === 10;
  }

  // Basic international validation (minimum 8 digits)
  return digits.length >= 8;
}

/**
 * Checks if a value is a number
 * @param value - Value to check
 */
export function isNumber(value: unknown): boolean {
  return !isNaN(parseFloat(value as string)) && isFinite(value as number);
}

/**
 * Checks if a value is empty (null, undefined, empty string, empty array, or empty object)
 * @param value - Value to check
 */
export function isEmpty(value: unknown): boolean {
  if (value === null || value === undefined) {
    return true;
  }

  if (typeof value === 'string') {
    return value.trim() === '';
  }

  if (Array.isArray(value)) {
    return value.length === 0;
  }

  if (typeof value === 'object') {
    return Object.keys(value).length === 0;
  }

  return false;
}

/* -------------------------------------------------------------------------- */
/* Password validation (reusable, structured)                                  */
/* -------------------------------------------------------------------------- */

export interface PasswordPolicy {
  min: number;
  max: number;
  requireUpper: boolean;
  requireLower: boolean;
  requireNumber: boolean;
  requireSpecial: boolean;
  forbidWhitespace: boolean;
}

export type PasswordErrorCode =
  | 'tooShort'
  | 'tooLong'
  | 'missingUpper'
  | 'missingLower'
  | 'missingNumber'
  | 'missingSpecial'
  | 'hasWhitespace';

const DEFAULT_PASSWORD_POLICY: PasswordPolicy = {
  min: 8,
  max: 64,
  requireUpper: true,
  requireLower: true,
  requireNumber: true,
  requireSpecial: false,
  forbidWhitespace: true,
};

export function validatePassword(
  password: string,
  policyOverrides?: Partial<PasswordPolicy>
): { valid: boolean; errors: PasswordErrorCode[] } {
  const policy: PasswordPolicy = { ...DEFAULT_PASSWORD_POLICY, ...(policyOverrides || {}) };
  const errors: PasswordErrorCode[] = [];

  if (password.length < policy.min) errors.push('tooShort');
  if (password.length > policy.max) errors.push('tooLong');
  if (policy.forbidWhitespace && /\s/.test(password)) errors.push('hasWhitespace');
  if (policy.requireUpper && !/[A-Z]/.test(password)) errors.push('missingUpper');
  if (policy.requireLower && !/[a-z]/.test(password)) errors.push('missingLower');
  if (policy.requireNumber && !/\d/.test(password)) errors.push('missingNumber');
  if (policy.requireSpecial && !/[!@#$%^&*(),.?":{}|<>\-_[\]`~;'/\\+=]/.test(password)) {
    errors.push('missingSpecial');
  }

  return { valid: errors.length === 0, errors };
}

export function mapPasswordErrorsToI18nKeys(errors: PasswordErrorCode[]): string[] {
  // Keys are relative to 'auth' namespace: passwordErrors.*
  return errors.map(code => `passwordErrors.${code}`);
}

/* -------------------------------------------------------------------------- */
/* Identifier validation (variable names)                                      */
/* -------------------------------------------------------------------------- */

/**
 * Checks if a string is a valid identifier consisting of letters, digits, and underscores only.
 * Empty string is considered invalid for final validation (allowed during typing elsewhere).
 */
export function isValidIdentifier(name: string): boolean {
  if (typeof name !== 'string') return false;
  if (name.length === 0) return false;
  // Must start with a letter, followed by letters/digits/underscores
  return /^[A-Za-z][A-Za-z0-9_]*$/.test(name);
}

/**
 * Sanitizes an input to a safe identifier: keep only [A-Za-z0-9_], collapse underscores.
 * Does not enforce non-empty or first-character rules.
 */
export function sanitizeIdentifier(input: string): string {
  let s = String(input ?? '');
  // Remove invalid characters
  s = s.replace(/[^A-Za-z0-9_]/g, '_');
  // Collapse consecutive underscores
  s = s.replace(/_+/g, '_');
  // Ensure starts with a letter: strip leading non-letters
  s = s.replace(/^[^A-Za-z]+/, '');
  return s;
}

/**
 * Ensure identifier uniqueness within a given list by appending numeric suffixes.
 * - exclude: an optional existing name to ignore (e.g., current row being edited)
 */
export function ensureUniqueIdentifier(base: string, existing: string[], exclude?: string): string {
  const used = new Set(
    existing
      .filter(Boolean)
      .map(s => String(s))
      .filter(name => (exclude ? name !== exclude : true))
  );
  if (!base) return base;
  if (!used.has(base)) return base;
  let i = 2;
  let candidate = `${base}_${i}`;
  while (used.has(candidate)) {
    i += 1;
    candidate = `${base}_${i}`;
  }
  return candidate;
}

/* -------------------------------------------------------------------------- */
/* DB column validation                                                        */
/* -------------------------------------------------------------------------- */

export const RESERVED_DB_COLUMN_NAMES: ReadonlySet<string> = new Set([
  'id',
  'uuid',
  'created_time',
  'updated_time',
  'add',
  'all',
  'alter',
  'and',
  'as',
  'asc',
  'auto_increment',
  'autocommit',
  'between',
  'bit',
  'blob',
  'boolean',
  'by',
  'case',
  'char',
  'check',
  'column',
  'commit',
  'create',
  'cross',
  'date',
  'datetime',
  'decimal',
  'default',
  'delete',
  'desc',
  'distinct',
  'double',
  'drop',
  'else',
  'enum',
  'exists',
  'float',
  'foreign',
  'from',
  'full',
  'group',
  'having',
  'in',
  'index',
  'inner',
  'insert',
  'int',
  'integer',
  'intersect',
  'into',
  'is',
  'join',
  'left',
  'like',
  'lock',
  'not',
  'null',
  'numeric',
  'on',
  'or',
  'order',
  'outer',
  'primary',
  'references',
  'right',
  'rollback',
  'savepoint',
  'select',
  'set',
  'table',
  'text',
  'then',
  'time',
  'timestamp',
  'transaction',
  'union',
  'unique',
  'unlock',
  'update',
  'values',
  'varchar',
  'when',
  'where',
]);

export function isInvalidDbColumnName(name: string): boolean {
  const n = (name || '').trim();
  if (!n) return true;
  return !/^[a-z][a-z0-9_]*$/.test(n);
}

export function isReservedDbColumnName(name: string): boolean {
  const n = (name || '').trim().toLowerCase();
  if (!n) return false;
  return RESERVED_DB_COLUMN_NAMES.has(n);
}

export function getDuplicateDbColumnNames<T extends { name?: string | null }>(
  columns: readonly T[]
): ReadonlySet<string> {
  const counter = new Map<string, number>();
  columns.forEach(col => {
    const name = (col.name || '').trim().toLowerCase();
    if (!name) return;
    counter.set(name, (counter.get(name) || 0) + 1);
  });

  const duplicates = new Set<string>();
  counter.forEach((count, name) => {
    if (count > 1) duplicates.add(name);
  });
  return duplicates;
}

/* -------------------------------------------------------------------------- */
/* Table name validation (DB table identifiers)                               */
/* -------------------------------------------------------------------------- */

/** Error codes for strict table name validation */
export type TableNameErrorCode =
  | 'required'
  | 'tooLong'
  | 'invalidChars'
  | 'mustStartWithLowercaseLetter'
  | 'noConsecutiveUnderscores'
  | 'duplicate';

/**
 * Validate DB table name with strict rules:
 * - Only lowercase letters (a-z), digits (0-9), and underscores
 * - Must start with a lowercase letter
 * - No consecutive underscores
 * - Max length: 63 characters
 */
export function isValidTableName(name: string): boolean {
  if (typeof name !== 'string') return false;
  const s = name;
  if (s.length === 0) return false; // required
  if (s.length > 63) return false; // length limit
  if (!/^[a-z]/.test(s)) return false; // must start with lowercase letter
  if (!/^[a-z0-9_]+$/.test(s)) return false; // allowed chars only
  if (/__/.test(s)) return false; // no consecutive underscores
  return true;
}

/**
 * Collect detailed error codes for invalid table names to drive i18n messages.
 */
export function getTableNameValidationErrors(name: string): TableNameErrorCode[] {
  const errors: TableNameErrorCode[] = [];
  const s = String(name ?? '');
  if (s.length === 0) {
    errors.push('required');
    return errors;
  }
  if (s.length > 63) errors.push('tooLong');
  if (!/^[a-z]/.test(s)) errors.push('mustStartWithLowercaseLetter');
  // Check invalid characters first to help users correct broader issues
  if (!/^[a-z0-9_]+$/.test(s)) errors.push('invalidChars');
  // Check consecutive underscores
  if (/__/.test(s)) errors.push('noConsecutiveUnderscores');
  return errors;
}

/* -------------------------------------------------------------------------- */
/* Name validation (display names, titles)                                     */
/* -------------------------------------------------------------------------- */

/**
 * Validates a user-facing name with Unicode support.
 * Rules:
 * - Length between 2 and 32 Unicode code points
 * - Allowed characters: letters (Unicode), numbers (Unicode), underscore, hyphen,
 *   and optionally spaces.
 * - Must contain at least one non-space character when spaces are allowed.
 */
export interface NameValidationOptions {
  /** Whether spaces are allowed; defaults to true */
  allowSpace?: boolean;
}

export function isValidNameInput(
  name: string,
  options: NameValidationOptions = { allowSpace: true }
): boolean {
  if (typeof name !== 'string') return false;

  // Measure length by Unicode code points (handles surrogate pairs correctly)
  const codePoints = Array.from(name);
  const len = codePoints.length;
  if (len < 2 || len > 32) return false;

  const allowSpace = options.allowSpace ?? true;
  // Avoid Unicode property escapes for broader runtime compatibility
  const isAllowedChar = (ch: string): boolean => {
    const cp = ch.codePointAt(0) as number;
    // ASCII digits
    if (cp >= 0x30 && cp <= 0x39) return true;
    // ASCII uppercase letters
    if (cp >= 0x41 && cp <= 0x5a) return true;
    // ASCII lowercase letters
    if (cp >= 0x61 && cp <= 0x7a) return true;
    // underscore or hyphen
    if (ch === '_' || ch === '-' || ch === '.') return true;
    // space (when allowed)
    if (allowSpace && ch === ' ') return true;
    // CJK Unified Ideographs (common Chinese)
    if ((cp >= 0x4e00 && cp <= 0x9fff) || (cp >= 0x3400 && cp <= 0x4dbf)) return true;
    return false;
  };

  if (!Array.from(name).every(isAllowedChar)) return false;

  // If spaces are allowed, ensure the name is not only whitespace
  if (allowSpace) {
    const nonSpace = name.replace(/\s+/gu, '');
    if (nonSpace.length === 0) return false;
  }

  return true;
}

/** Error codes for name input validation */
export type NameErrorCode = 'required' | 'tooShort' | 'tooLong' | 'invalidChars' | 'onlySpaces';

/**
 * Returns error codes for a display name according to unified rules.
 * Use with i18n to show precise messages in UI.
 */
export function getNameValidationErrors(
  name: string,
  options: NameValidationOptions = { allowSpace: true }
): NameErrorCode[] {
  const allowSpace = options.allowSpace ?? true;
  const errors: NameErrorCode[] = [];

  const len = Array.from(name ?? '').length;
  const hasNonSpace = (name ?? '').trim().length > 0;
  // Same allowed-char check as in isValidNameInput
  const isAllowedChar = (ch: string): boolean => {
    const cp = ch.codePointAt(0) as number;
    if (cp >= 0x30 && cp <= 0x39) return true; // 0-9
    if (cp >= 0x41 && cp <= 0x5a) return true; // A-Z
    if (cp >= 0x61 && cp <= 0x7a) return true; // a-z
    if (ch === '_' || ch === '-' || ch === '.') return true; // underscore, hyphen
    if (allowSpace && ch === ' ') return true; // space
    if ((cp >= 0x4e00 && cp <= 0x9fff) || (cp >= 0x3400 && cp <= 0x4dbf)) return true; // CJK
    return false;
  };

  if (len === 0) {
    errors.push('required');
    return errors;
  }
  if (len < 2) errors.push('tooShort');
  if (len > 32) errors.push('tooLong');
  if (!Array.from(name).every(isAllowedChar)) errors.push('invalidChars');
  if (allowSpace && !hasNonSpace) errors.push('onlySpaces');
  return errors;
}
