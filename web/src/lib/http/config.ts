// HTTP configuration management for multiple environments and services

import { NODE_ENV, API_URL, AUTH_API_URL, UPLOAD_API_URL, MARKET_API_URL } from '@/lib/config';

export interface ApiEndpoint {
  name: string;
  baseURL: string;
  timeout?: number;
  version?: string;
}

export interface HttpConfig {
  default: ApiEndpoint;
  endpoints: Record<string, ApiEndpoint>;
  retryAttempts: number;
  retryDelay: number;
  globalTimeout: number;
}

// Default timeouts for different service types
const DEFAULT_TIMEOUTS = {
  default: 30000,
  auth: 15000,
  upload: 1800000,
  market: 60000,
} as const;

// Generate endpoint URLs based on main API URL and environment variables
function generateEndpointUrls(baseApiUrl: string) {
  // Use environment variables first, fall back to derived URLs

  return {
    main: baseApiUrl,
    auth: AUTH_API_URL || baseApiUrl,
    upload: UPLOAD_API_URL || baseApiUrl,
    market: MARKET_API_URL || baseApiUrl,
  };
}

// Environment-specific configurations
function createConfig(environment: string): HttpConfig {
  const endpointUrls = generateEndpointUrls(API_URL);
  const baseConfig = {
    default: {
      name: 'main',
      baseURL: endpointUrls.main,
      timeout: DEFAULT_TIMEOUTS.default,
      version: '',
    },
    endpoints: {
      main: {
        name: 'main',
        baseURL: endpointUrls.main,
        timeout: DEFAULT_TIMEOUTS.default,
        version: '',
      },
      auth: {
        name: 'auth',
        baseURL: endpointUrls.auth,
        timeout: DEFAULT_TIMEOUTS.auth,
        version: '',
      },
      upload: {
        name: 'upload',
        baseURL: endpointUrls.upload,
        timeout: DEFAULT_TIMEOUTS.upload,
        version: '',
      },
      market: {
        name: 'market',
        baseURL: endpointUrls.market,
        timeout: DEFAULT_TIMEOUTS.market,
        version: '',
      },
    },
  };

  // Environment-specific overrides
  switch (environment) {
    case 'production':
      return {
        ...baseConfig,
        retryAttempts: 0,
        retryDelay: 500,
        globalTimeout: 30000,
      };
    case 'test':
      return {
        ...baseConfig,
        default: {
          ...baseConfig.default,
          baseURL: 'http://localhost:8080',
          timeout: 10000,
        },
        endpoints: {
          ...baseConfig.endpoints,
          main: {
            ...baseConfig.endpoints.main,
            baseURL: 'http://localhost:8080',
            timeout: 10000,
          },
          auth: {
            ...baseConfig.endpoints.auth,
            baseURL: 'http://localhost:8081',
            timeout: 5000,
          },
        },
        retryAttempts: 0,
        retryDelay: 100,
        globalTimeout: 15000,
      };
    case 'development':
    default:
      return {
        ...baseConfig,
        retryAttempts: 0,
        retryDelay: 1000,
        globalTimeout: 60000,
      };
  }
}

// Get current environment
export function getCurrentEnvironment(): string {
  return NODE_ENV || 'development';
}

// Get configuration for current environment
export function getHttpConfig(): HttpConfig {
  const env = getCurrentEnvironment();
  return createConfig(env);
}

// Get specific endpoint configuration
export function getEndpointConfig(endpointName?: string): ApiEndpoint {
  const config = getHttpConfig();

  if (!endpointName) {
    return config.default;
  }

  const endpoint = config.endpoints[endpointName];
  if (!endpoint) {
    console.warn(`Endpoint '${endpointName}' not found, falling back to default`);
    return config.default;
  }

  return endpoint;
}

// Build full URL with version support
export function buildApiUrl(path: string, endpointName?: string): string {
  const endpoint = getEndpointConfig(endpointName);
  const basePath = endpoint.version ? `/${endpoint.version}` : '';
  const cleanPath = path.startsWith('/') ? path : `/${path}`;

  return `${endpoint.baseURL}${basePath}${cleanPath}`;
}

// Environment variable overrides (deprecated - use environment variables directly)
export function applyEnvOverrides(config: HttpConfig): HttpConfig {
  console.warn(
    'applyEnvOverrides is deprecated. Configure URLs via environment variables instead.'
  );
  return config;
}
