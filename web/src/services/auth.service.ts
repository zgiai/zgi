import { BaseService } from '@/lib/http/services';
import type {
  Account,
  CasdoorConsumeTicketRequest,
  CasdoorConsumeTicketResponse,
  LoginRequest,
  RegisterVerifyRequest,
  RegisterFinishRequest,
  VerifyRequest,
  SetupRequest,
  ActivationCheckResponse,
  CompleteRegistrationResponse,
  ForgotPasswordInitResponse,
  ResetPasswordResponse,
  RegisterInitResponse,
  LoginResponse,
  ErrorResponse,
  PhoneCodeRequest,
  PhoneCodeResponse,
  PhoneCheckRequest,
  PhoneCheckResponse,
  PhoneVerifyRequest,
  PhoneVerifyResponse,
  PhoneRegisterRequest,
  PhoneLoginRequest,
} from './types/auth';
import type { User, SystemFeatures, SetupStatus } from '@/services/types/auth';
import type { ApiResponseData, BusinessError } from './types/common';
import { deleteCookie, setRawCookie } from '@/utils/cookie';
import {
  CLIENT_CACHE_KEYS,
  LEGACY_CLIENT_CACHE_COOKIE_KEYS,
  PROFILE_CLIENT_CACHE_TTL_MS,
  SYSTEM_FEATURES_CLIENT_CACHE_TTL_MS,
  readClientCacheWithLegacyCookie,
  writeClientCache,
} from '@/utils/client-cache';
import { ENABLE_ROOT_COOKIE_TOKEN_SYNC, ROOT_COOKIE_DOMAIN } from '@/lib/config';
import { sessionManager } from '@/lib/auth/session-manager';

type SystemFeaturesResponse = ApiResponseData<{ features: SystemFeatures }>;

let inFlightSystemFeaturesRequest: Promise<SystemFeaturesResponse> | null = null;

// Enhanced authentication service using unified architecture
export class AuthenticationService extends BaseService {
  private readonly idTokenCookieName = 'idToken';
  private readonly tokenCookieDays = 30;

  private get tokenCookieEnabled(): boolean {
    return ENABLE_ROOT_COOKIE_TOKEN_SYNC && ROOT_COOKIE_DOMAIN.length > 0;
  }

  private getCookieDomainForCurrentHost(): string | undefined {
    if (typeof window === 'undefined' || !this.tokenCookieEnabled) {
      return undefined;
    }

    const normalizedDomain = ROOT_COOKIE_DOMAIN.replace(/^\./, '').toLowerCase();
    const hostname = window.location.hostname.toLowerCase();

    if (hostname === normalizedDomain || hostname.endsWith(`.${normalizedDomain}`)) {
      return ROOT_COOKIE_DOMAIN;
    }

    console.info('[SSO Service] root cookie domain skipped for current host', {
      configuredDomain: ROOT_COOKIE_DOMAIN,
      hostname,
    });

    return undefined;
  }

  private syncIdTokenCookie(idToken?: string): void {
    if (typeof window === 'undefined') {
      return;
    }

    const cookieDomain = this.getCookieDomainForCurrentHost();
    const options = {
      domain: cookieDomain,
      sameSite: 'Lax' as const,
    };

    if (!idToken) {
      deleteCookie(this.idTokenCookieName);
      if (cookieDomain) {
        deleteCookie(this.idTokenCookieName, { domain: cookieDomain });
      }
      return;
    }

    setRawCookie(this.idTokenCookieName, idToken, this.tokenCookieDays, options);

    if (typeof window !== 'undefined') {
      console.info('[SSO Service] idToken cookie synced', {
        cookieDomain: cookieDomain || 'current-host',
        cookiePresent: document.cookie.includes(`${this.idTokenCookieName}=`),
      });
    }
  }

  private persistTokens(accessToken: string, refreshToken?: string): void {
    sessionManager.setSession(
      {
        accessToken,
        refreshToken,
      },
      { type: 'SIGNED_IN' }
    );
  }

  constructor() {
    super({
      endpoint: 'auth',
      basePath: '/console/api',
    });
  }

  // Handle business logic errors for authentication services
  private handleBusinessError(error: unknown, _context: string = ''): never {
    if (error && typeof error === 'object' && 'businessError' in error) {
      const { code, message } = (error as BusinessError).businessError;
      const authError = new Error(message || '');
      (authError as unknown as BusinessError).businessError = {
        code,
        message: message || '',
      };
      throw authError;
    }

    throw error;
  }

  // Login with enhanced error handling
  async login(credentials: LoginRequest): Promise<{ access_token: string; user: Account }> {
    const response = await this.request<ApiResponseData<LoginResponse>>(
      'post',
      '/login',
      credentials,
      {
        skipAuth: true,
        retryAttemptsOverride: 0,
      }
    );

    // Handle business logic success/error after HTTP success
    // Check if code indicates success (0 as number or '0' as string)
    const isSuccess = response.code === '0' || response.code === '0';
    if (!isSuccess) {
      // This is a business logic error, handle it specially
      const error = new Error(response.message || 'Login failed');
      (error as unknown as BusinessError).businessError = {
        code: response.code || '',
        message: response.message || '',
      };
      this.handleBusinessError(error, 'Login');
    }

    const access_token = response.data?.data?.access_token;
    const refresh_token = response.data?.data?.refresh_token;
    const account = response.data?.data?.account;

    if (!access_token) {
      throw new Error('Invalid login response structure');
    }

    this.persistTokens(access_token, refresh_token);

    return {
      access_token,
      user: account,
    };
  }

  async consumeCasdoorTicket(
    payload: CasdoorConsumeTicketRequest
  ): Promise<{ access_token: string; refresh_token?: string; user: Account }> {
    if (typeof window !== 'undefined') {
      console.info('[SSO Service] consumeCasdoorTicket request start', {
        ticketPreview: `${payload.ticket.slice(0, 8)}...`,
      });
    }

    const response = await this.request<ApiResponseData<CasdoorConsumeTicketResponse>>(
      'post',
      '/sso/casdoor/consume-ticket',
      payload,
      {
        skipAuth: true,
      }
    );

    if (response.code !== '0') {
      const error = new Error(response.message || 'SSO login failed');
      (error as unknown as BusinessError).businessError = {
        code: response.code || '',
        message: response.message || '',
      };
      this.handleBusinessError(error, 'SSO');
    }

    const accessToken = response.data?.data?.access_token;
    const refreshToken = response.data?.data?.refresh_token;
    const account = response.data?.data?.account;
    const ssoProvider = response.data?.data?.sso?.provider;
    const idToken = response.data?.data?.sso?.id_token;

    if (typeof window !== 'undefined') {
      console.info('[SSO Service] consumeCasdoorTicket response received', {
        code: response.code,
        result: response.data?.result,
        hasAccessToken: Boolean(accessToken),
        hasRefreshToken: Boolean(refreshToken),
        hasAccount: Boolean(account),
        ssoProvider,
        hasIdToken: Boolean(idToken),
      });
    }

    if (!accessToken || !account) {
      if (typeof window !== 'undefined') {
        console.error('[SSO Service] invalid response structure', {
          hasAccessToken: Boolean(accessToken),
          hasAccount: Boolean(account),
        });
      }
      throw new Error('Invalid SSO response structure');
    }

    this.persistTokens(accessToken, refreshToken);
    this.syncIdTokenCookie(ssoProvider === 'casdoor' ? idToken : undefined);

    if (typeof window !== 'undefined') {
      console.info('[SSO Service] tokens persisted', {
        authTokenInStorage: Boolean(sessionManager.getAccessToken()),
        refreshTokenInStorage: Boolean(sessionManager.getRefreshToken()),
        idTokenCookieSynced: ssoProvider === 'casdoor' && Boolean(idToken),
      });
    }

    return {
      access_token: accessToken,
      refresh_token: refreshToken,
      user: account,
    };
  }

  // Logout with token cleanup
  async logout(): Promise<void> {
    const accessToken = sessionManager.getAccessToken();

    try {
      await this.request<void>('post', '/logout', undefined, {
        skipAuth: true,
        skipErrorHandling: true,
        retryAttemptsOverride: 0,
        ...(accessToken ? { headers: { Authorization: `Bearer ${accessToken}` } } : {}),
      });
    } catch (error) {
      // Log the error but don't throw - logout should always succeed locally
      console.warn('Server logout failed (this is usually not critical):', error);
    }

    // Clean up local storage
    sessionManager.clearSession({ type: 'SIGNED_OUT' });
    this.syncIdTokenCookie();
  }

  // Refresh token
  async refreshToken(): Promise<{ access_token: string }> {
    const currentRefreshToken = sessionManager.getRefreshToken();
    if (!currentRefreshToken) {
      console.warn('🚫 No refresh token available, cannot refresh');
      throw new Error('No refresh token available');
    }

    const response = await this.request<{ access_token: string }>('post', '/refresh-token');

    if (response.access_token) {
      sessionManager.setSession(
        {
          accessToken: response.access_token,
          refreshToken: currentRefreshToken,
        },
        { type: 'TOKEN_REFRESHED' }
      );
    }

    return response;
  }

  // Send forgot password email and return server token
  async forgotPassword(email: string, language?: string): Promise<ForgotPasswordInitResponse> {
    const response = await this.request<
      ErrorResponse | ApiResponseData<{ result: string; data: string }>
    >(
      'post',
      '/forgot-password',
      { email, language },
      {
        skipAuth: true,
        retryAttemptsOverride: 0,
      }
    );

    // Handle error response
    if ('code' in response && response.code !== '0') {
      const error = new Error(response.message || 'Forgot password failed');
      (error as unknown as BusinessError).businessError = {
        code: response.code || '',
        message: response.message || '',
      };
      throw error;
    }

    if ('data' in response) {
      return {
        result: response?.data?.result || '',
        token: response?.data?.data || '',
      };
    }

    throw new Error('Unexpected response format');
  }

  // Reset password
  async resetPassword(data: {
    email: string;
    new_password: string;
    password_confirm: string;
    token: string;
    language?: string;
  }): Promise<ResetPasswordResponse> {
    const response = await this.request<
      ApiResponseData<{
        result: string;
        data?: { access_token?: string; refresh_token?: string };
      }>
    >('post', '/forgot-password/resets', data, { skipAuth: true });
    return response?.data || {};
  }

  // Get user profile
  async getProfile(useCache: boolean = true): Promise<User> {
    // Check client local cache first
    try {
      if (useCache && typeof window !== 'undefined') {
        const cached = readClientCacheWithLegacyCookie<User>({
          key: CLIENT_CACHE_KEYS.profile,
          legacyCookieKey: LEGACY_CLIENT_CACHE_COOKIE_KEYS.profile,
          ttlMs: PROFILE_CLIENT_CACHE_TTL_MS,
        });
        if (cached) {
          return cached;
        }
      }
    } catch {
      // ignore client cache errors and fall back to network
    }

    const response = await this.request<ApiResponseData<User>>('get', '/account/profile');

    if (response.code !== '0') {
      throw new Error(response.message || 'Failed to get profile');
    }

    const data = response.data;

    // Write client local cache for 3 days
    try {
      if (typeof window !== 'undefined') {
        writeClientCache(CLIENT_CACHE_KEYS.profile, data, PROFILE_CLIENT_CACHE_TTL_MS);
        deleteCookie(LEGACY_CLIENT_CACHE_COOKIE_KEYS.profile);
      }
    } catch {
      // silently ignore
    }

    return data;
  }

  // Verify registration code – handle wrapped or bare responses
  async verifyRegister(data: RegisterVerifyRequest): Promise<{ is_valid: boolean; email: string }> {
    interface VerifyPayload {
      email: string;
      is_valid: boolean;
    }

    const response = await this.request<ApiResponseData<VerifyPayload>>(
      'post',
      '/register/validity',
      data,
      { skipAuth: true, retryAttemptsOverride: 0 }
    );

    if (response.code !== '0') {
      const error = new Error(response.message || 'Verification failed');
      (error as unknown as BusinessError).businessError = {
        code: response.code || '',
        message: response.message || '',
      };
      throw error;
    }

    return response.data;
  }

  // Finish registration
  async finishRegister(
    data: RegisterFinishRequest
  ): Promise<{ access_token: string; refresh_token?: string; account?: Account }> {
    interface FinishPayload {
      data: {
        access_token: string;
        refresh_token?: string;
        account?: Account;
      };
      result: string;
    }

    const response = await this.request<ApiResponseData<FinishPayload>>(
      'post',
      '/register/finish',
      data,
      { skipAuth: true }
    );

    if (response.code !== '0') {
      throw new Error(response.message || 'Registration completion failed');
    }

    return response.data.data;
  }

  // Initial registration – request a verification code
  async startRegister(email: string, language: string): Promise<RegisterInitResponse> {
    const response = await this.request<ApiResponseData<RegisterInitResponse>>(
      'post',
      '/register',
      { email, language },
      { skipAuth: true, retryAttemptsOverride: 0 }
    );

    // Any non-zero code indicates a business error
    if ('code' in response && response.code !== '0') {
      const error = new Error(response.message || 'Registration failed');
      (error as unknown as BusinessError).businessError = {
        code: response.code || '',
        message: response.message || '',
      };
      throw error;
    }

    // New standard response format
    if (!('data' in response) || !response.data) {
      throw new Error('Malformed registration response');
    }

    const {
      result = '',
      data: token = '',
      server_time,
      resend_available_at,
      resend_after_seconds,
    } = response.data as {
      result?: string;
      data?: string;
      server_time?: number;
      resend_available_at?: number;
      resend_after_seconds?: number;
    };

    return {
      result,
      token,
      server_time,
      resend_available_at,
      resend_after_seconds,
    };
  }

  // Verify forgot password code
  async verifyForgotPassword(
    data: RegisterVerifyRequest
  ): Promise<ApiResponseData<{ is_valid: boolean; email: string }>> {
    return this.request<ApiResponseData<{ is_valid: boolean; email: string }>>(
      'post',
      '/forgot-password/validity',
      data,
      { skipAuth: true, retryAttemptsOverride: 0 }
    );
  }

  // Get system features
  async getSystemFeatures(useCache: boolean = true): Promise<SystemFeaturesResponse> {
    // Client local cache first
    try {
      if (useCache && typeof window !== 'undefined') {
        const cached = readClientCacheWithLegacyCookie<SystemFeaturesResponse>({
          key: CLIENT_CACHE_KEYS.systemFeatures,
          legacyCookieKey: LEGACY_CLIENT_CACHE_COOKIE_KEYS.systemFeatures,
          ttlMs: SYSTEM_FEATURES_CLIENT_CACHE_TTL_MS,
        });
        if (cached) {
          return cached;
        }
      }
    } catch {
      // ignore client cache errors and fall back to network
    }

    if (!inFlightSystemFeaturesRequest) {
      const request = (async () => {
        const fresh = await this.request<SystemFeaturesResponse>(
          'get',
          '/system-features',
          undefined,
          {
            skipAuth: true,
          }
        );

        // Cache response for 3 days.
        try {
          if (typeof window !== 'undefined') {
            writeClientCache(
              CLIENT_CACHE_KEYS.systemFeatures,
              fresh,
              SYSTEM_FEATURES_CLIENT_CACHE_TTL_MS
            );
            deleteCookie(LEGACY_CLIENT_CACHE_COOKIE_KEYS.systemFeatures);
          }
        } catch {
          // silently ignore
        }

        return fresh;
      })();

      inFlightSystemFeaturesRequest = request;
      request.then(
        () => {
          if (inFlightSystemFeaturesRequest === request) {
            inFlightSystemFeaturesRequest = null;
          }
        },
        () => {
          if (inFlightSystemFeaturesRequest === request) {
            inFlightSystemFeaturesRequest = null;
          }
        }
      );
    }

    return inFlightSystemFeaturesRequest;
  }

  // Get setup status
  async getSetupStatus(): Promise<ApiResponseData<SetupStatus>> {
    return this.request<ApiResponseData<SetupStatus>>('get', '/setup', undefined, {
      skipAuth: true,
    });
  }

  // Setup system
  async setup(data: SetupRequest): Promise<{ result: string }> {
    return this.request<{ result: string }>('post', '/setup', data, { skipAuth: true });
  }

  // Check activation
  async checkActivate(params: {
    email: string;
    token: string;
    workspace_id?: string;
  }): Promise<ActivationCheckResponse> {
    const response = await this.request<ApiResponseData<ActivationCheckResponse>>(
      'get',
      '/activate/check',
      undefined,
      {
        params,
        skipAuth: true,
      }
    );
    return response.data;
  }

  // Activate account
  async activate(data: {
    token: string;
    name: string;
    interface_language: string;
    timezone: string;
  }): Promise<void> {
    return this.request('post', '/activate', data, { skipAuth: true });
  }

  // Verify email/phone
  async verify(request: VerifyRequest): Promise<CompleteRegistrationResponse> {
    return this.request<CompleteRegistrationResponse>('post', '/verify', request, {
      skipAuth: true,
    });
  }

  // Complete registration
  async completeRegistration(data: { token: string }): Promise<CompleteRegistrationResponse> {
    return this.request<CompleteRegistrationResponse>('post', '/complete-registration', data, {
      skipAuth: true,
    });
  }

  // Get current token
  getToken(): string | null {
    return sessionManager.getAccessToken();
  }

  // Check if user is authenticated
  isAuthenticated(): boolean {
    return !!this.getToken();
  }

  async verifyCode(data: {
    email: string;
    code: string;
    token: string;
    type?: string;
  }): Promise<{ is_valid: boolean; email: string }> {
    const endpoint = data.type === 'reset' ? '/forgot-password/validity' : '/register/validity';
    return this.request<{ is_valid: boolean; email: string }>('post', endpoint, data, {
      skipAuth: true,
    });
  }

  async verifyRegistrationCode(data: {
    email: string;
    code: string;
    token: string;
  }): Promise<{ is_valid: boolean; email: string }> {
    return this.verifyCode({ ...data, type: 'register' });
  }

  async verifyResetCode(data: {
    email: string;
    code: string;
    token: string;
  }): Promise<{ is_valid: boolean; email: string }> {
    return this.verifyCode({ ...data, type: 'reset' });
  }

  // Check if email is already registered
  async checkEmail(email: string): Promise<{ is_registered: boolean; data?: unknown }> {
    const response = await this.request<ApiResponseData<{ is_registered: boolean }>>(
      'post',
      '/email/check',
      { email },
      { skipAuth: true }
    );

    if ('code' in response && response.code !== '0') {
      const error = new Error(response.message || 'Email check failed');
      (error as unknown as BusinessError).businessError = {
        code: response.code || '',
        message: response.message || '',
      };
      throw error;
    }

    return {
      is_registered: response.data?.is_registered || false,
      data: response.data,
    };
  }

  // Phone authentication methods
  async checkPhone(data: PhoneCheckRequest): Promise<PhoneCheckResponse> {
    const response = await this.request<ApiResponseData<PhoneCheckResponse>>(
      'post',
      '/phone/check',
      data,
      { skipAuth: true }
    );
    if (response.code !== '0') {
      const error = new Error(response.message || 'Phone check failed');
      (error as unknown as BusinessError).businessError = {
        code: response.code || '',
        message: response.message || '',
      };
      this.handleBusinessError(error, 'Phone check');
    }
    return response.data;
  }

  async sendPhoneCode(data: PhoneCodeRequest): Promise<PhoneCodeResponse> {
    const response = await this.request<ApiResponseData<PhoneCodeResponse>>(
      'post',
      '/phone/code',
      data,
      { skipAuth: true }
    );
    if (response.code !== '0') {
      const error = new Error(response.message || 'Failed to send verification code');
      (error as unknown as BusinessError).businessError = {
        code: response.code || '',
        message: response.message || '',
      };
      this.handleBusinessError(error, 'Phone code');
    }
    return response.data;
  }

  async verifyPhoneCode(data: PhoneVerifyRequest): Promise<PhoneVerifyResponse> {
    const response = await this.request<ApiResponseData<PhoneVerifyResponse>>(
      'post',
      '/phone/code/verify',
      data,
      { skipAuth: true }
    );
    if (response.code !== '0') {
      const error = new Error(response.message || 'Verification failed');
      (error as unknown as BusinessError).businessError = {
        code: response.code || '',
        message: response.message || '',
      };
      this.handleBusinessError(error, 'Phone verify');
    }
    return response.data;
  }

  async phoneRegister(
    data: PhoneRegisterRequest
  ): Promise<{ access_token: string; refresh_token?: string; account?: Account }> {
    const response = await this.request<ApiResponseData<LoginResponse>>(
      'post',
      '/phone/register',
      data,
      { skipAuth: true }
    );
    if (response.code !== '0') {
      const error = new Error(response.message || 'Registration failed');
      (error as unknown as BusinessError).businessError = {
        code: response.code || '',
        message: response.message || '',
      };
      this.handleBusinessError(error, 'Phone register');
    }
    const access_token = response.data?.data?.access_token;
    const refresh_token = response.data?.data?.refresh_token;
    const account = response.data?.data?.account;

    if (!access_token) {
      throw new Error('Invalid phone registration response structure');
    }
    this.persistTokens(access_token, refresh_token);
    return { access_token, refresh_token, account };
  }

  async phoneLogin(
    data: PhoneLoginRequest
  ): Promise<{ access_token: string; refresh_token?: string; account?: Account }> {
    const response = await this.request<ApiResponseData<LoginResponse>>(
      'post',
      '/phone/login',
      data,
      { skipAuth: true }
    );
    if (response.code !== '0') {
      const error = new Error(response.message || 'Login failed');
      (error as unknown as BusinessError).businessError = {
        code: response.code || '',
        message: response.message || '',
      };
      this.handleBusinessError(error, 'Phone login');
    }
    const access_token = response.data?.data?.access_token;
    const refresh_token = response.data?.data?.refresh_token;
    const account = response.data?.data?.account;

    if (!access_token) {
      throw new Error('Invalid phone login response structure');
    }
    this.persistTokens(access_token, refresh_token);
    return { access_token, refresh_token, account };
  }
}

// Export singleton instance for new service
export const authenticationService = new AuthenticationService();

// Export as default for backward compatibility
export default authenticationService;

// Legacy service export for existing code compatibility
export const authService = {
  // Core authentication methods
  login: (data: LoginRequest) => authenticationService.login(data),
  logout: () => authenticationService.logout(),
  refreshToken: () => authenticationService.refreshToken(),
  forgotPassword: (data: { email: string; language?: string }) =>
    authenticationService.forgotPassword(data.email, data.language),
  resetPassword: (data: {
    email: string;
    new_password: string;
    password_confirm: string;
    token: string;
    language?: string;
  }) => authenticationService.resetPassword(data),

  // Profile methods
  getProfile: () => authenticationService.getProfile(),

  // Registration flow
  verifyRegister: (data: RegisterVerifyRequest) => authenticationService.verifyRegister(data),
  finishRegister: (data: RegisterFinishRequest) => authenticationService.finishRegister(data),
  verifyForgotPassword: (data: RegisterVerifyRequest) =>
    authenticationService.verifyForgotPassword(data),

  // System methods
  getSystemFeatures: (useCache: boolean = true) =>
    authenticationService.getSystemFeatures(useCache),
  getSetupStatus: () => authenticationService.getSetupStatus(),
  setup: (data: SetupRequest) => authenticationService.setup(data),

  // Activation methods
  checkActivate: (params: { email: string; token: string; workspace_id?: string }) =>
    authenticationService.checkActivate(params),
  activate: (data: { token: string; name: string; interface_language: string; timezone: string }) =>
    authenticationService.activate(data),

  // New methods
  startRegister: (data: { email: string; language: string }) =>
    authenticationService.startRegister(data.email, data.language),

  // Phone authentication legacy wrapper
  checkPhone: (data: PhoneCheckRequest) => authenticationService.checkPhone(data),
  sendPhoneCode: (data: PhoneCodeRequest) => authenticationService.sendPhoneCode(data),
  verifyPhoneCode: (data: PhoneVerifyRequest) => authenticationService.verifyPhoneCode(data),
  phoneRegister: (data: PhoneRegisterRequest) => authenticationService.phoneRegister(data),
  phoneLogin: (data: PhoneLoginRequest) => authenticationService.phoneLogin(data),
};
