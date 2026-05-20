export interface HttpHeaderKV {
  key: string;
  value: string;
}

/** Text body data item */
export interface HttpRequestBodyTextItem {
  id: string;
  type: 'text';
  key: string;
  value: string;
}

/** File body data item with variable reference */
export interface HttpRequestBodyFileItem {
  id: string;
  type: 'file';
  key: string;
  value: string;
  /** Variable reference path, e.g. ['sys', 'files'] */
  file: string[];
}

/** Body data item - can be text or file */
export type HttpRequestBodyDataItem = HttpRequestBodyTextItem | HttpRequestBodyFileItem;
export type HttpRequestBody =
  | { type: 'none'; data: [] }
  | { type: 'raw-text'; data: HttpRequestBodyDataItem[] }
  | { type: 'json'; data: HttpRequestBodyDataItem[] }
  | { type: 'form-data'; data: HttpRequestBodyDataItem[] };

/**
 * Timeout configuration for HTTP requests
 */
export interface TimeoutConfig {
  /** Maximum allowed connect timeout (seconds), 0 = no limit */
  max_connect_timeout: number;
  /** Maximum allowed read timeout (seconds), 0 = no limit */
  max_read_timeout: number;
  /** Maximum allowed write timeout (seconds), 0 = no limit */
  max_write_timeout: number;
  /** Actual connect timeout (seconds) */
  connect: number;
  /** Actual read timeout (seconds) */
  read: number;
  /** Actual write timeout (seconds) */
  write: number;
}

/**
 * Retry configuration for failed HTTP requests
 */
export interface RetryConfig {
  /** Whether retry is enabled */
  retry_enabled: boolean;
  /** Maximum retry attempts (0 = no retry) */
  max_retries: number;
  /** Interval between retries (milliseconds) */
  retry_interval: number;
}

/**
 * Error handling strategy for HTTP request node
 * - 'none': No special error handling (current behavior)
 * - 'default-value': Use user-defined default values on error
 * - 'fail-branch': Route to a fail branch handle on error
 */
export type ErrorStrategy = 'none' | 'default-value' | 'fail-branch';

/**
 * Default value item for error handling
 */
export interface ErrorDefaultValueItem {
  key: 'body' | 'status_code' | 'headers';
  type: 'string' | 'number' | 'object';
  value: string;
}

/** Default error values for default-value strategy */
export const DEFAULT_ERROR_VALUES: ErrorDefaultValueItem[] = [
  { key: 'body', type: 'string', value: '' },
  { key: 'status_code', type: 'number', value: '200' },
  { key: 'headers', type: 'object', value: '{}' },
];

/** Default timeout configuration */
export const DEFAULT_TIMEOUT_CONFIG: TimeoutConfig = {
  max_connect_timeout: 0,
  max_read_timeout: 0,
  max_write_timeout: 0,
  connect: 10,
  read: 60,
  write: 10,
};

/** Default retry configuration */
export const DEFAULT_RETRY_CONFIG: RetryConfig = {
  retry_enabled: false,
  max_retries: 3,
  retry_interval: 1000,
};

export interface HttpRequestNodeData {
  type: 'http-request';
  title: string;
  desc: string;
  method: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH' | 'HEAD';
  url: string;
  params: HttpHeaderKV[];
  headers: HttpHeaderKV[];
  body: HttpRequestBody;
  /** Timeout configuration */
  timeout: TimeoutConfig;
  /** Retry configuration */
  retry_config: RetryConfig;
  /** Error handling strategy */
  error_strategy?: ErrorStrategy;
  /** Default values when error_strategy is 'default-value' */
  default_value?: ErrorDefaultValueItem[];
  isInLoop: boolean;
  isInIteration: boolean;
}

export const DEFAULT_HTTP_REQUEST_NODE_DATA: HttpRequestNodeData = {
  type: 'http-request',
  title: 'HTTP Request',
  desc: '',
  method: 'GET',
  url: '',
  params: [],
  headers: [],
  body: { type: 'none', data: [] },
  timeout: DEFAULT_TIMEOUT_CONFIG,
  retry_config: DEFAULT_RETRY_CONFIG,
  error_strategy: 'none',
  default_value: [],
  isInLoop: false,
  isInIteration: false,
};

import type { ValidationResult, ValidationError } from '../common/validation';

export const checkValid = (data: HttpRequestNodeData): ValidationResult => {
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  if (!data.url || data.url.trim() === '') {
    errors.push({ code: 'httpRequest.validation.urlRequired' });
  }

  // headers
  if (Array.isArray(data.headers)) {
    data.headers.forEach((h, idx) => {
      if (!h.key || h.key.trim() === '') {
        errors.push({
          code: 'httpRequest.validation.headerKeyRequired',
          params: { index: idx + 1 },
        });
      }
    });
  }

  // params
  if (Array.isArray(data.params)) {
    data.params.forEach((p, idx) => {
      if (!p.key || p.key.trim() === '') {
        errors.push({
          code: 'httpRequest.validation.paramKeyRequired',
          params: { index: idx + 1 },
        });
      }
    });
  }

  // body
  if (data.body) {
    const items = Array.isArray(data.body.data) ? data.body.data : [];

    if (data.body.type === 'form-data') {
      // form-data: requires key for each item
      items.forEach((item, idx) => {
        if (!item.key || item.key.trim() === '') {
          errors.push({
            code: 'httpRequest.validation.bodyItemKeyRequired',
            params: { index: idx + 1 },
          });
        }
      });
    } else if (data.body.type === 'json') {
      // json: value should be valid JSON syntax
      items.forEach((item, idx) => {
        if (item.value && item.value.trim()) {
          // Skip validation if value contains template variables like {{#...#}}
          const hasTemplateVar = /\{\{#[^#]+#\}\}/.test(item.value);
          if (!hasTemplateVar) {
            try {
              JSON.parse(item.value);
            } catch {
              errors.push({
                code: 'httpRequest.validation.bodyItemInvalidJson',
                params: { index: idx + 1 },
              });
            }
          }
        } else {
          errors.push({
            code: 'httpRequest.validation.bodyItemValueRequired',
            params: { index: idx + 1 },
          });
        }
      });
    } else if (data.body.type === 'raw-text') {
      // raw-text: value should not be empty
      items.forEach((item, idx) => {
        if (!item.value || item.value.trim() === '') {
          errors.push({
            code: 'httpRequest.validation.bodyItemValueRequired',
            params: { index: idx + 1 },
          });
        }
      });
    }
  }

  return { isValid: errors.length === 0, errors, warnings };
};
