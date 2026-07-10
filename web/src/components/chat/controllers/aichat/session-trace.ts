export type AIChatSendTraceSource = 'keyboard' | 'button' | 'unknown';

export interface AIChatSendTraceContext {
  traceId: string;
  source: AIChatSendTraceSource;
  startedAt: number;
}

const AICHAT_SESSION_TRACE_PREFIX = '[AIChatSessionTrace]';
let traceSequence = 0;
let instanceSequence = 0;
const traceDocumentStartedAt = Date.now();
const traceDocumentId = `doc-${traceDocumentStartedAt.toString(36)}-${Math.random()
  .toString(36)
  .slice(2, 8)}`;

export function createAIChatTraceInstanceId(scope: string) {
  instanceSequence += 1;
  return `${scope}-${instanceSequence.toString(36)}`;
}

export function createAIChatSendTraceContext(
  source: AIChatSendTraceSource
): AIChatSendTraceContext {
  traceSequence += 1;
  return {
    traceId: `${Date.now().toString(36)}-${traceSequence.toString(36)}`,
    source,
    startedAt: Date.now(),
  };
}

export function logAIChatSessionTrace(
  stage: string,
  details: Record<string, unknown> = {},
  context?: AIChatSendTraceContext | null
) {
  const entry = {
    timestamp: new Date().toISOString(),
    documentId: traceDocumentId,
    documentAgeMs: Date.now() - traceDocumentStartedAt,
    navigationType:
      typeof performance === 'undefined'
        ? null
        : ((
            performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming | undefined
          )?.type ?? null),
    visibilityState: typeof document === 'undefined' ? null : document.visibilityState,
    pagePath: typeof window === 'undefined' ? null : window.location.pathname,
    stage,
    traceId: context?.traceId ?? null,
    source: context?.source ?? null,
    elapsedMs: context ? Date.now() - context.startedAt : null,
    ...details,
  };
  // Intentionally visible in production builds while this issue is being diagnosed.
  // eslint-disable-next-line no-console
  console.info(AICHAT_SESSION_TRACE_PREFIX, JSON.stringify(entry));
}
