import { captureRouterTransitionStart, initializeZGIReporter } from '@/lib/observability';

initializeZGIReporter();

export const onRouterTransitionStart = captureRouterTransitionStart;
