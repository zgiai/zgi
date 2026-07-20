// Token management: storage, refresh, and validation

import type { AxiosInstance } from 'axios';
import { AxiosError } from 'axios';
import { willTokenExpireSoon } from '@/utils/jwt';
import { withBasePath } from '@/lib/config';
import { captureError } from '@/lib/observability';
import { consumePendingLogoutRedirect } from '@/utils/logout-redirect';
import { sessionManager } from '@/lib/auth/session-manager';
import { isLogoutInProgress, markAuthRedirectInProgress } from '@/lib/auth/logout-state';
import type { ExtendedRequestConfig } from './types';

export class StaleTokenRefreshError extends Error {
  code = 'ERR_AUTH_REFRESH_STALE';

  constructor(message: string) {
    super(message);
    this.name = 'StaleTokenRefreshError';
  }
}

export function isStaleTokenRefreshError(error: unknown): error is StaleTokenRefreshError {
  return (
    error instanceof StaleTokenRefreshError ||
    (typeof error === 'object' &&
      error !== null &&
      (error as { code?: string }).code === 'ERR_AUTH_REFRESH_STALE')
  );
}

function isTransientNetworkError(error: unknown): boolean {
  if (!(error instanceof AxiosError)) return false;

  const message = error.message.toLowerCase();
  return (
    !error.response ||
    error.code === 'NETWORK_ERROR' ||
    error.code === 'ERR_NETWORK' ||
    error.code === 'ECONNABORTED' ||
    error.code === 'ETIMEDOUT' ||
    message === 'network error' ||
    message.includes('timeout')
  );
}

let sharedRefreshPromise: Promise<string> | null = null;

/**
 * Manages authentication tokens: storage, refresh, and validation.
 * Handles concurrent refresh requests with a browser-context shared promise.
 */
export class TokenManager {
  constructor(
    private instance: AxiosInstance,
    private endpointName: string
  ) {}

  getAuthToken(): string | null {
    return sessionManager.getAccessToken();
  }

  getRefreshToken(): string | null {
    return sessionManager.getRefreshToken();
  }

  clearAuthData(): void {
    sessionManager.clearSession({ type: 'SIGNED_OUT' });
  }

  canUseToken(token: string): boolean {
    return !isLogoutInProgress() && this.getAuthToken() === token;
  }

  private assertRefreshSessionCurrent(refreshToken: string): void {
    if (isLogoutInProgress()) {
      throw new StaleTokenRefreshError('Token refresh discarded because logout is in progress');
    }

    if (this.getRefreshToken() !== refreshToken) {
      throw new StaleTokenRefreshError('Token refresh discarded because the auth session changed');
    }
  }

  redirectToLogin(): void {
    if (typeof window !== 'undefined' && !window.location.pathname.includes('/login')) {
      const pendingLogoutRedirect = consumePendingLogoutRedirect();
      if (pendingLogoutRedirect) {
        markAuthRedirectInProgress();
        window.location.replace(pendingLogoutRedirect);
        return;
      }
      const currentPath = window.location.pathname + window.location.search;
      const loginUrl = withBasePath(`/login?redirect=${encodeURIComponent(currentPath)}`);
      markAuthRedirectInProgress();
      window.location.replace(loginUrl);
    }
  }

  /**
   * Ensures a valid token is available.
   * Proactively refreshes if token is missing or about to expire.
   */
  async ensureValidToken(): Promise<string | null> {
    if (isLogoutInProgress()) {
      return null;
    }

    let token = this.getAuthToken();
    const refreshToken = this.getRefreshToken();

    // No tokens at all - user needs to login
    if (!token && !refreshToken) {
      return null;
    }

    // Have access token but lost refresh token - clear and force login
    if (token && !refreshToken) {
      return token;
    }

    // If access token is missing OR will expire soon, proactively refresh
    if (!token || (typeof window !== 'undefined' && willTokenExpireSoon(token, 2))) {
      try {
        const refreshedToken = await this.getOrRefreshToken();
        if (refreshedToken && this.canUseToken(refreshedToken)) {
          return refreshedToken;
        }

        token = this.getAuthToken();
      } catch (error) {
        if (isStaleTokenRefreshError(error)) {
          token = this.getAuthToken();
          if (token && this.canUseToken(token)) {
            return token;
          }
        }

        throw error;
      }
    }

    return token;
  }

  /**
   * Gets current token or triggers a refresh if needed.
   * Handles concurrent refresh requests.
   */
  async getOrRefreshToken(): Promise<string | null> {
    if (isLogoutInProgress()) {
      throw new StaleTokenRefreshError('Token refresh skipped because logout is in progress');
    }

    // Share one refresh request across all HTTP clients/endpoints in this browser context.
    if (sharedRefreshPromise) {
      return sharedRefreshPromise;
    }

    const refreshToken = this.getRefreshToken();
    if (!refreshToken) {
      this.clearAuthData();
      this.redirectToLogin();
      throw new Error('No refresh token available');
    }

    this.assertRefreshSessionCurrent(refreshToken);

    const refreshPromise = this.performTokenRefresh(refreshToken);
    sharedRefreshPromise = refreshPromise;

    try {
      return await refreshPromise;
    } catch (error) {
      if (isStaleTokenRefreshError(error)) {
        throw error;
      }

      if (isTransientNetworkError(error)) {
        throw error;
      }

      this.clearAuthData();
      this.redirectToLogin();
      throw error;
    } finally {
      if (sharedRefreshPromise === refreshPromise) {
        sharedRefreshPromise = null;
      }
    }
  }

  /**
   * Performs the actual token refresh API call.
   */
  private async performTokenRefresh(refreshToken: string): Promise<string> {
    try {
      const response = await this.instance.post(
        '/console/api/refresh-token',
        { refresh_token: refreshToken },
        {
          skipAuth: true,
          skipErrorHandling: true,
          timeout: 10000,
        } as ExtendedRequestConfig
      );

      const data = response.data;
      const newToken = data?.access_token || data?.data?.access_token;
      const newRefreshToken = data?.refresh_token || data?.data?.refresh_token;

      this.assertRefreshSessionCurrent(refreshToken);

      if (!newToken) {
        throw new Error('Invalid refresh response: missing access_token');
      }

      this.assertRefreshSessionCurrent(refreshToken);

      sessionManager.setSession(
        {
          accessToken: newToken,
          refreshToken: newRefreshToken ?? refreshToken,
        },
        { type: 'TOKEN_REFRESHED' }
      );

      return newToken;
    } catch (error) {
      if (isStaleTokenRefreshError(error)) {
        throw error;
      }

      this.assertRefreshSessionCurrent(refreshToken);

      // Handle specific refresh token errors
      if (error instanceof AxiosError) {
        const status = error.response?.status;
        const code =
          (error.response?.data as { code?: string; errorCode?: string })?.code ||
          (error.response?.data as { errorCode?: string })?.errorCode;

        if (status === 401 || status === 403 || (status === 400 && code === '212012')) {
          throw new Error('Refresh token is invalid or expired');
        }

        if (isTransientNetworkError(error)) {
          throw error;
        }
      }

      const status = error instanceof AxiosError ? error.response?.status : undefined;
      captureError(error, 'auth.token.refresh_failed', {
        tags: { endpoint: this.endpointName },
        attributes: {
          http: {
            path: '/console/api/refresh-token',
            method: 'POST',
            status: status || 0,
          },
          auth: { reason: 'refresh_request_failed' },
        },
      });
      throw error;
    }
  }

  get isCurrentlyRefreshing(): boolean {
    return Boolean(sharedRefreshPromise);
  }
}
