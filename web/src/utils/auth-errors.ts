export interface ErrorWithBusinessError extends Error {
  businessError?: {
    code?: string;
    message?: string;
  };
  response?: {
    data?: {
      code?: string;
      message?: string;
      data?: unknown;
    };
  };
}

export type AuthBusinessErrorDescriptionKey =
  | 'businessErrors.invalidParameter'
  | 'businessErrors.invalidCredentials'
  | 'businessErrors.accountNotFound'
  | 'businessErrors.invalidLoginStatus'
  | 'businessErrors.registrationNotAllowed'
  | 'businessErrors.accountSuspended'
  | 'businessErrors.accountFrozen'
  | 'businessErrors.accountBanned'
  | 'businessErrors.emailRequired'
  | 'businessErrors.passwordRequired'
  | 'businessErrors.emailAlreadyExists'
  | 'businessErrors.invalidEmailFormat'
  | 'businessErrors.passwordTooWeak'
  | 'businessErrors.passwordMismatch'
  | 'businessErrors.invalidVerificationCode'
  | 'businessErrors.verificationCodeExpired'
  | 'businessErrors.tooManyAttempts'
  | 'businessErrors.loginTooManyAttempts'
  | 'businessErrors.sendCodeTooManyAttempts'
  | 'businessErrors.verifyCodeTooManyAttempts'
  | 'businessErrors.emailServiceUnavailable'
  | 'businessErrors.emailCheckFailed'
  | 'businessErrors.resetVerificationExpired'
  | 'businessErrors.registerTokenExpired'
  | 'businessErrors.resetPasswordFailed';

export type AuthBusinessErrorContext =
  | 'login'
  | 'forgotPassword'
  | 'verification'
  | 'register'
  | 'resetPassword';

interface AuthBusinessErrorDescriptionOptions {
  suppressInvalidCredentials?: boolean;
  context?: AuthBusinessErrorContext;
}

const AUTH_BUSINESS_ERROR_DESCRIPTION_KEYS: Record<string, AuthBusinessErrorDescriptionKey> = {
  account_not_found: 'businessErrors.accountNotFound',
  '199001': 'businessErrors.invalidParameter',
  '401001': 'businessErrors.invalidCredentials',
  '401002': 'businessErrors.invalidLoginStatus',
  '401003': 'businessErrors.accountSuspended',
  '403007': 'businessErrors.registrationNotAllowed',
  '101002': 'businessErrors.invalidEmailFormat',
  '101005': 'businessErrors.invalidVerificationCode',
  '201011': 'businessErrors.accountFrozen',
  '201012': 'businessErrors.accountNotFound',
  '201016': 'businessErrors.accountBanned',
  '201017': 'businessErrors.invalidCredentials',
  '201018': 'businessErrors.loginTooManyAttempts',
  '201001': 'businessErrors.emailAlreadyExists',
  '201002': 'businessErrors.passwordRequired',
  '201003': 'businessErrors.emailAlreadyExists',
  '201004': 'businessErrors.invalidEmailFormat',
  '201005': 'businessErrors.passwordMismatch',
  '202001': 'businessErrors.invalidVerificationCode',
  '202002': 'businessErrors.verificationCodeExpired',
  '201008': 'businessErrors.tooManyAttempts',
  '503002': 'businessErrors.emailServiceUnavailable',
};

const AUTH_CONTEXT_ERROR_DESCRIPTION_KEYS: Partial<
  Record<AuthBusinessErrorContext, Partial<Record<string, AuthBusinessErrorDescriptionKey>>>
> = {
  login: {
    '199001': 'businessErrors.invalidCredentials',
  },
  forgotPassword: {
    '201008': 'businessErrors.sendCodeTooManyAttempts',
  },
  register: {
    '201008': 'businessErrors.sendCodeTooManyAttempts',
    '401002': 'businessErrors.registerTokenExpired',
  },
  verification: {
    '201008': 'businessErrors.verifyCodeTooManyAttempts',
    '401002': 'businessErrors.resetVerificationExpired',
  },
  resetPassword: {
    '401002': 'businessErrors.resetVerificationExpired',
  },
};

export function getAuthBusinessError(error: unknown): ErrorWithBusinessError | null {
  if (!error || typeof error !== 'object') {
    return null;
  }

  return error as ErrorWithBusinessError;
}

export function getAuthBusinessErrorCode(error: unknown): string | undefined {
  const businessError = getAuthBusinessError(error);
  return businessError?.businessError?.code || businessError?.response?.data?.code;
}

export function getAuthBusinessErrorMessage(error: unknown): string | undefined {
  const businessError = getAuthBusinessError(error);
  return (
    businessError?.businessError?.message ||
    businessError?.response?.data?.message ||
    businessError?.message ||
    undefined
  );
}

export function getAuthBusinessErrorData(error: unknown): unknown {
  return getAuthBusinessError(error)?.response?.data?.data;
}

export function getAuthBusinessErrorDescriptionKey(
  error: unknown,
  options: AuthBusinessErrorDescriptionOptions = {}
): AuthBusinessErrorDescriptionKey | undefined {
  const code = getAuthBusinessErrorCode(error);
  if (options.suppressInvalidCredentials && (code === '401001' || code === '201017')) {
    return undefined;
  }

  if (!code) {
    return undefined;
  }

  const contextKey = options.context
    ? AUTH_CONTEXT_ERROR_DESCRIPTION_KEYS[options.context]?.[code]
    : undefined;

  return contextKey ?? AUTH_BUSINESS_ERROR_DESCRIPTION_KEYS[code];
}
