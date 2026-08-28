import type { NextRequest } from 'next/server';
import { proxyIntegrationOAuthRequest } from '@/server/integration-oauth-proxy';

interface OAuthProxyContext {
  params: Promise<{ path: string[] }>;
}

async function proxy(request: NextRequest, context: OAuthProxyContext): Promise<Response> {
  const { path } = await context.params;
  return proxyIntegrationOAuthRequest(request, path);
}

export const dynamic = 'force-dynamic';

export async function GET(request: NextRequest, context: OAuthProxyContext): Promise<Response> {
  return proxy(request, context);
}

export async function POST(request: NextRequest, context: OAuthProxyContext): Promise<Response> {
  return proxy(request, context);
}
