'use client';

import { use } from 'react';
import { ConnectionManagementPage } from '@/components/connectors/connections/connection-management-page';

export default function PersonalConnectionPage({
  params,
  searchParams,
}: {
  params: Promise<{ connectionId: string }>;
  searchParams: Promise<{ rename?: string }>;
}) {
  const { connectionId } = use(params);
  const { rename } = use(searchParams);
  return (
    <ConnectionManagementPage
      connectionId={connectionId}
      enabled
      renameInitiallyOpen={rename === '1'}
    />
  );
}
