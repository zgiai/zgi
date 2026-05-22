import {
  captureRouterTransitionStart,
  initializeSentryClient,
} from '@/lib/sentry/client';

initializeSentryClient();

export const onRouterTransitionStart = captureRouterTransitionStart;
