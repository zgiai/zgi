import type * as SentryClient from './sdk';

type SentryModule = typeof SentryClient;

type SentryScope = SentryClient.SentryScope;

let sentryPromise: Promise<SentryModule | null> | null = null;
let initialized = false;

function readPublicSentryEnv(key: string): string | undefined {
  if (typeof window !== 'undefined' && window.__ENV__) {
    const value = window.__ENV__[key];
    if (typeof value === 'string' && value.length > 0) return value;
  }

  switch (key) {
    case 'NEXT_PUBLIC_SENTRY_DSN':
      return process.env.NEXT_PUBLIC_SENTRY_DSN;
    case 'NEXT_PUBLIC_SENTRY_ENVIRONMENT':
      return process.env.NEXT_PUBLIC_SENTRY_ENVIRONMENT;
    case 'NEXT_PUBLIC_DEPLOY_ENV':
      return process.env.NEXT_PUBLIC_DEPLOY_ENV;
    case 'NEXT_PUBLIC_SENTRY_REPLAY_ENABLED':
      return process.env.NEXT_PUBLIC_SENTRY_REPLAY_ENABLED;
    case 'NEXT_PUBLIC_SENTRY_TRACES_SAMPLE_RATE':
      return process.env.NEXT_PUBLIC_SENTRY_TRACES_SAMPLE_RATE;
    case 'NEXT_PUBLIC_SENTRY_REPLAYS_SESSION_SAMPLE_RATE':
      return process.env.NEXT_PUBLIC_SENTRY_REPLAYS_SESSION_SAMPLE_RATE;
    case 'NEXT_PUBLIC_SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE':
      return process.env.NEXT_PUBLIC_SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE;
    default:
      return undefined;
  }
}

function isEnabled(value: string | undefined): boolean {
  return value === 'true' || value === '1' || value === 'yes';
}

function readSampleRate(key: string): number {
  const value = readPublicSentryEnv(key);
  if (!value) return 0;

  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return 0;

  return Math.min(Math.max(parsed, 0), 1);
}

async function loadSentry(): Promise<SentryModule | null> {
  if (typeof window === 'undefined') return null;

  const dsn = readPublicSentryEnv('NEXT_PUBLIC_SENTRY_DSN');
  if (!dsn) return null;

  if (!sentryPromise) {
    sentryPromise = import('./sdk')
      .then(sentryClient => {
        if (!initialized) {
          sentryClient.initSentryClient({
            dsn,
            environment:
              readPublicSentryEnv('NEXT_PUBLIC_SENTRY_ENVIRONMENT') ||
              readPublicSentryEnv('NEXT_PUBLIC_DEPLOY_ENV') ||
              process.env.NODE_ENV,
            replayEnabled: isEnabled(readPublicSentryEnv('NEXT_PUBLIC_SENTRY_REPLAY_ENABLED')),
            tracesSampleRate: readSampleRate('NEXT_PUBLIC_SENTRY_TRACES_SAMPLE_RATE'),
            replaysSessionSampleRate: readSampleRate(
              'NEXT_PUBLIC_SENTRY_REPLAYS_SESSION_SAMPLE_RATE'
            ),
            replaysOnErrorSampleRate: readSampleRate(
              'NEXT_PUBLIC_SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE'
            ),
          });
          initialized = true;
        }

        return sentryClient;
      })
      .catch(error => {
        console.warn('Failed to load Sentry client:', error);
        sentryPromise = null;
        return null;
      });
  }

  return sentryPromise;
}

export function initializeSentryClient(): void {
  void loadSentry();
}

export function captureException(
  error: unknown,
  configureScope?: (scope: SentryScope) => void
): void {
  void loadSentry().then(Sentry => {
    if (!Sentry) return;

    if (configureScope) {
      Sentry.withScope(scope => {
        configureScope(scope as SentryScope);
        Sentry.captureException(error);
      });
      return;
    }

    Sentry.captureException(error);
  });
}

export function captureMessage(
  message: string,
  configureScope?: (scope: SentryScope) => void
): void {
  void loadSentry().then(Sentry => {
    if (!Sentry) return;

    if (configureScope) {
      Sentry.withScope(scope => {
        configureScope(scope as SentryScope);
        Sentry.captureMessage(message);
      });
      return;
    }

    Sentry.captureMessage(message);
  });
}

export function withScope(callback: (scope: SentryScope) => void): void {
  void loadSentry().then(Sentry => {
    Sentry?.withScope(scope => callback(scope as SentryScope));
  });
}

export function captureRouterTransitionStart(...args: unknown[]): void {
  void loadSentry().then(Sentry => {
    const captureTransition = Sentry?.captureRouterTransitionStart as
      | ((...transitionArgs: unknown[]) => void)
      | undefined;
    captureTransition?.(...args);
  });
}
