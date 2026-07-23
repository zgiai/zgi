export type ZGIEventKind = 'error' | 'event';
export type ZGIEventLevel = 'debug' | 'info' | 'warning' | 'error' | 'fatal';

export interface ZGIEvent {
  name: string;
  kind: ZGIEventKind;
  level: ZGIEventLevel;
  error?: unknown;
  tags?: Record<string, string | number | boolean>;
  attributes?: Record<string, unknown>;
  occurredAt: string;
}

export interface CaptureOptions {
  level?: ZGIEventLevel;
  tags?: Record<string, string | number | boolean>;
  attributes?: Record<string, unknown>;
}

/** Provider-neutral extension point for Sentry, OpenTelemetry, or customer adapters. */
export interface Reporter {
  readonly name: string;
  report(event: Readonly<ZGIEvent>): void | Promise<void>;
  flush?(): void | Promise<void>;
  onRouterTransitionStart?(...args: unknown[]): void;
}
