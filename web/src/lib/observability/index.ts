import { SentryReporter } from './reporters/sentry';
import type { CaptureOptions, Reporter } from './types';
import { ZGIReporter } from './zgi-reporter';

export type { CaptureOptions, Reporter, ZGIEvent } from './types';
export { NoopReporter, ZGIReporter } from './zgi-reporter';

export const zgiReporter = new ZGIReporter();

let initialized = false;

function readPublicReporterEnv(
  key: 'NEXT_PUBLIC_ZGI_REPORTERS' | 'NEXT_PUBLIC_SENTRY_DSN'
): string | undefined {
  if (typeof window !== 'undefined' && window.__ENV__) {
    const value = window.__ENV__[key];
    if (typeof value === 'string' && value.trim()) return value;
  }

  switch (key) {
    case 'NEXT_PUBLIC_ZGI_REPORTERS':
      return process.env.NEXT_PUBLIC_ZGI_REPORTERS;
    case 'NEXT_PUBLIC_SENTRY_DSN':
      return process.env.NEXT_PUBLIC_SENTRY_DSN;
  }
}

function selectedBuiltInReporters(): Set<string> {
  const configured = readPublicReporterEnv('NEXT_PUBLIC_ZGI_REPORTERS');
  if (configured) {
    const reporters = new Set(
      configured
        .split(/[\s,]+/)
        .map(item => item.trim().toLowerCase())
        .filter(Boolean)
    );
    if (reporters.has('none')) return new Set();
    return reporters;
  }

  return readPublicReporterEnv('NEXT_PUBLIC_SENTRY_DSN') ? new Set(['sentry']) : new Set();
}

export function initializeZGIReporter(): void {
  if (initialized || typeof window === 'undefined') return;
  initialized = true;

  const selected = selectedBuiltInReporters();
  if (selected.has('sentry') && readPublicReporterEnv('NEXT_PUBLIC_SENTRY_DSN')) {
    const sentryReporter = new SentryReporter();
    zgiReporter.register(sentryReporter);
    sentryReporter.initialize();
  }
}

/** Registers a customer adapter. Duplicate adapter names are ignored. */
export function registerReporter(reporter: Reporter): void {
  zgiReporter.register(reporter);
}

export function captureError(
  error: unknown,
  name = 'zgi.error',
  options: CaptureOptions = {}
): void {
  initializeZGIReporter();
  zgiReporter.report({
    name,
    kind: 'error',
    level: options.level || 'error',
    error,
    tags: options.tags,
    attributes: options.attributes,
    occurredAt: new Date().toISOString(),
  });
}

export function captureEvent(name: string, options: CaptureOptions = {}): void {
  initializeZGIReporter();
  zgiReporter.report({
    name,
    kind: 'event',
    level: options.level || 'info',
    tags: options.tags,
    attributes: options.attributes,
    occurredAt: new Date().toISOString(),
  });
}

export function captureRouterTransitionStart(...args: unknown[]): void {
  initializeZGIReporter();
  zgiReporter.onRouterTransitionStart(...args);
}
