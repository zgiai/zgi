import type { NextRequest } from 'next/server';
import { proxyIntegrationProviderInstallRequest } from '@/server/integration-oauth-proxy';

interface ProviderInstallProxyContext {
  params: Promise<{ path: string[] }>;
}

async function proxy(
  request: NextRequest,
  context: ProviderInstallProxyContext
): Promise<Response> {
  const { path } = await context.params;
  return proxyIntegrationProviderInstallRequest(request, path);
}

export const dynamic = 'force-dynamic';

export async function GET(
  request: NextRequest,
  context: ProviderInstallProxyContext
): Promise<Response> {
  return proxy(request, context);
}

export async function POST(
  request: NextRequest,
  context: ProviderInstallProxyContext
): Promise<Response> {
  return proxy(request, context);
}
