'use client';

import { use } from 'react';
import { ConnectorDetailPage } from '@/components/connectors/detail/connector-detail-page';
import { ConnectorDeepLink } from '@/components/connectors/detail/connector-deep-link';

export default function ConnectorPage({
  params,
  searchParams,
}: {
  params: Promise<{ connectorId: string }>;
  searchParams: Promise<{ pending_setup_id?: string; connection_id?: string }>;
}) {
  const { connectorId } = use(params);
  const { pending_setup_id, connection_id } = use(searchParams);
  return (
    <>
      <ConnectorDetailPage connectorId={connectorId} enabled />
      <ConnectorDeepLink
        connectorId={connectorId}
        pendingID={pending_setup_id}
        connectionID={connection_id}
      />
    </>
  );
}
