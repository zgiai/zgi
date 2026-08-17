import { Suspense } from 'react';
import { IntegrationOAuthResultClient } from '@/components/integrations/oauth-result-client';

export default function IntegrationOAuthResultPage() {
  return (
    <Suspense fallback={null}>
      <IntegrationOAuthResultClient />
    </Suspense>
  );
}
