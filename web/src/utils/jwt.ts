// JWT utility functions for token management

interface JWTPayload {
  exp?: number; // Expiration time in seconds
  iat?: number; // Issued at time in seconds
  sub?: string; // Subject (user ID)
  iss?: string; // Issuer
  aud?: string | string[]; // Audience
  nbf?: number; // Not before time
  jti?: string; // JWT ID
  // Allow additional claims with specific types
  [key: string]: string | number | boolean | string[] | undefined;
}

/**
 * Parse JWT token payload without verification (client-side only)
 * Note: This is for reading token expiration only, not for security validation
 */
export function parseJWTPayload(token: string): JWTPayload | null {
  try {
    // JWT structure: header.payload.signature
    const parts = token.split('.');
    if (parts.length !== 3) {
      return null;
    }

    // Decode the payload (base64url)
    const payload = parts[1];
    // Add padding if needed
    const paddedPayload = payload + '='.repeat((4 - (payload.length % 4)) % 4);

    // Replace URL-safe characters
    const base64 = paddedPayload.replace(/-/g, '+').replace(/_/g, '/');

    // Decode and parse
    const decoded = atob(base64);
    return JSON.parse(decoded);
  } catch (error) {
    console.warn('Failed to parse JWT payload:', error);
    return null;
  }
}

/**
 * Check if JWT token is expired
 */
export function isTokenExpired(token: string | null): boolean {
  if (!token) {
    return true;
  }

  const payload = parseJWTPayload(token);
  if (!payload || !payload.exp) {
    // If we can't parse the expiration, consider it expired for safety
    return true;
  }

  // Convert expiration time to milliseconds and compare with current time
  const expirationTime = payload.exp * 1000;
  const currentTime = Date.now();

  return currentTime >= expirationTime;
}

/**
 * Get token expiration time in human readable format
 */
export function getTokenExpirationTime(token: string | null): string | null {
  if (!token) {
    return null;
  }

  const payload = parseJWTPayload(token);
  if (!payload || !payload.exp) {
    return null;
  }

  const expirationTime = new Date(payload.exp * 1000);
  return expirationTime.toLocaleString();
}

/**
 * Get remaining time until token expires
 */
export function getTokenRemainingTime(token: string | null): {
  minutes: number;
  seconds: number;
  isExpired: boolean;
} {
  if (!token) {
    return { minutes: 0, seconds: 0, isExpired: true };
  }

  const payload = parseJWTPayload(token);
  if (!payload || !payload.exp) {
    return { minutes: 0, seconds: 0, isExpired: true };
  }

  const expirationTime = payload.exp * 1000;
  const currentTime = Date.now();
  const remainingTime = expirationTime - currentTime;

  if (remainingTime <= 0) {
    return { minutes: 0, seconds: 0, isExpired: true };
  }

  const minutes = Math.floor(remainingTime / (1000 * 60));
  const seconds = Math.floor((remainingTime % (1000 * 60)) / 1000);

  return { minutes, seconds, isExpired: false };
}

/**
 * Check if token will expire within the specified minutes
 */
export function willTokenExpireSoon(token: string | null, withinMinutes: number = 5): boolean {
  if (!token) {
    return true;
  }

  const { minutes, isExpired } = getTokenRemainingTime(token);

  return isExpired || minutes <= withinMinutes;
}
