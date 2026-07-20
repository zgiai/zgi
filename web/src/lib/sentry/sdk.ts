import * as Sentry from '@sentry/nextjs';
import { sanitizeSentryEvent } from '@/lib/observability/sentry-sanitize';

export interface SentryScope {
  setContext(name: string, context: Record<string, unknown>): void;
  setTag(key: string, value: string | number | boolean): void;
  setLevel(level: 'debug' | 'info' | 'warning' | 'error' | 'fatal'): void;
}

interface SentryClientOptions {
  dsn: string;
  environment: string;
  replayEnabled: boolean;
  tracesSampleRate: number;
  replaysSessionSampleRate: number;
  replaysOnErrorSampleRate: number;
}

let initialized = false;

export function initSentryClient(options: SentryClientOptions): void {
  if (initialized) return;

  Sentry.init({
    dsn: options.dsn,
    environment: options.environment,
    integrations: options.replayEnabled ? [Sentry.replayIntegration()] : [],
    tracesSampleRate: options.tracesSampleRate,
    enableLogs: false,
    replaysSessionSampleRate: options.replayEnabled ? options.replaysSessionSampleRate : 0,
    replaysOnErrorSampleRate: options.replayEnabled ? options.replaysOnErrorSampleRate : 0,
    sendDefaultPii: false,
    beforeSend: sanitizeSentryEvent,
  });
  initialized = true;
}

export const captureException = Sentry.captureException;
export const captureMessage = Sentry.captureMessage;
export const withScope = Sentry.withScope;
export const captureRouterTransitionStart = Sentry.captureRouterTransitionStart;
