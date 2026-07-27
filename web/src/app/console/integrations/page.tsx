'use client';

import { Suspense } from 'react';
import { useSearchParams } from 'next/navigation';
import {
  IntegrationConnectionCenter,
  type IntegrationConnectionCenterView,
} from '@/components/integrations/connection-center';
import { useSystemFeatures } from '@/hooks';

function connectionCenterView(value: string | null): IntegrationConnectionCenterView {
  switch (value) {
    case 'connected':
    case 'policies':
    case 'executions':
      return value;
    default:
      return 'available';
  }
}

function ConnectionCenterPageContent() {
  const searchParams = useSearchParams();
  const systemFeatures = useSystemFeatures();

  return (
    <IntegrationConnectionCenter
      enabled={Boolean(systemFeatures.data?.enable_external_integrations)}
      featureLoading={systemFeatures.isLoading}
      initialView={connectionCenterView(searchParams.get('view'))}
      initialIntegrationId={searchParams.get('integration_id') ?? undefined}
    />
  );
}

export default function ConnectionCenterPage() {
  return (
    <Suspense fallback={null}>
      <ConnectionCenterPageContent />
    </Suspense>
  );
}
