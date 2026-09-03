'use client';

import { use } from 'react';
import { ConnectorDetailPage } from '@/components/connectors/detail/connector-detail-page';

export default function ConnectorPage({ params }: { params: Promise<{ connectorId: string }> }) {
  const { connectorId } = use(params);
  return <ConnectorDetailPage connectorId={connectorId} enabled />;
}
