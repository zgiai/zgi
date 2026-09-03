interface ConnectorRequestError {
  businessError?: { code?: unknown };
  response?: { status?: number; data?: { code?: unknown; error_code?: unknown } };
}

function stringValue(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value.trim() : undefined;
}

export function connectorRequestErrorDetails(error: unknown): {
  code: string;
  retryable: boolean;
  outcomeUnknown: boolean;
  status?: number;
} {
  const candidate = (error ?? {}) as ConnectorRequestError;
  const status = candidate.response?.status;
  if (!status) {
    // A missing HTTP response cannot establish whether the server accepted the Key.
    return { code: 'transport_unconfirmed', retryable: true, outcomeUnknown: true };
  }
  const code =
    stringValue(candidate.response?.data?.error_code) ??
    stringValue(candidate.response?.data?.code) ??
    stringValue(candidate.businessError?.code) ??
    'request_failed';
  return {
    code,
    retryable:
      status >= 500 || code === 'connector_provider_unavailable' || code === 'provider_unavailable',
    outcomeUnknown: false,
    status,
  };
}
