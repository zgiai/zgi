'use client';

import { Suspense } from 'react';
import { ConnectorCenter } from '@/components/connectors/catalog/connector-center';

export default function ConnectorCenterPage() {
  return (
    <Suspense fallback={null}>
      <ConnectorCenter enabled />
    </Suspense>
  );
}
