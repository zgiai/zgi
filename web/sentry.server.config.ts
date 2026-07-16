// This file configures the initialization of Sentry on the server.
// The config you add here will be used whenever the server handles a request.
// https://docs.sentry.io/platforms/javascript/guides/nextjs/

import * as Sentry from '@sentry/nextjs';
import { sanitizeSentryEvent } from './src/lib/observability/sentry-sanitize';

const sentryDSN = process.env.SENTRY_DSN || process.env.NEXT_PUBLIC_SENTRY_DSN;
const configuredReporters = process.env.ZGI_REPORTERS || process.env.NEXT_PUBLIC_ZGI_REPORTERS;
const selectedReporters = configuredReporters
  ?.split(/[\s,]+/)
  .map(value => value.trim().toLowerCase())
  .filter(Boolean);
const sentryEnabled = Boolean(
  sentryDSN &&
    (!selectedReporters?.length ||
      (!selectedReporters.includes('none') && selectedReporters.includes('sentry')))
);

const sentryEnvironment =
  process.env.SENTRY_ENVIRONMENT ||
  process.env.NEXT_PUBLIC_SENTRY_ENVIRONMENT ||
  process.env.NEXT_PUBLIC_DEPLOY_ENV ||
  process.env.DEPLOY_ENV ||
  process.env.NODE_ENV;

function readSampleRate(value: string | undefined, fallback: number): number {
  if (!value) return fallback;

  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return fallback;

  return Math.min(Math.max(parsed, 0), 1);
}

const tracesSampleRate = readSampleRate(
  process.env.SENTRY_TRACES_SAMPLE_RATE || process.env.NEXT_PUBLIC_SENTRY_TRACES_SAMPLE_RATE,
  1
);

Sentry.init({
  dsn: sentryDSN,
  enabled: sentryEnabled,
  environment: sentryEnvironment,

  // Define how likely traces are sampled. Adjust this value in production, or use tracesSampler for greater control.
  tracesSampleRate,

  // Enable logs to be sent to Sentry
  enableLogs: true,

  // Keep default user PII disabled for open-source and self-hosted deployments.
  // https://docs.sentry.io/platforms/javascript/guides/nextjs/configuration/options/#sendDefaultPii
  sendDefaultPii: false,
  beforeSend: sanitizeSentryEvent,
});
