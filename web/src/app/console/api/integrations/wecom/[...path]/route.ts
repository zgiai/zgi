import type { NextRequest } from 'next/server';
import { proxyIntegrationWeComCallbackRequest } from '@/server/integration-oauth-proxy';

interface WeComCallbackProxyContext {
  params: Promise<{ path: string[] }>;
}

async function proxy(request: NextRequest, context: WeComCallbackProxyContext): Promise<Response> {
  const { path } = await context.params;
  return proxyIntegrationWeComCallbackRequest(request, path);
}

export const dynamic = 'force-dynamic';

export async function GET(
  request: NextRequest,
  context: WeComCallbackProxyContext
): Promise<Response> {
  return proxy(request, context);
}

export async function POST(
  request: NextRequest,
  context: WeComCallbackProxyContext
): Promise<Response> {
  return proxy(request, context);
}
