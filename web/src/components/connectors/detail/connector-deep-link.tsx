'use client';

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import { useT } from '@/i18n';
import { useConnector } from '@/hooks/connectors/use-connectors';
import { usePendingSetup } from '@/hooks/connectors/use-pending-setups';
import { connectorService } from '@/services/connector.service';
import { ConnectorCreateDialog } from '@/components/connectors/setup/connector-create-dialog';
import { ConnectorPageMessage } from '@/components/connectors/connector-page-state';

// Deep links only resolve owner-scoped resources. They never begin a scan,
// submit credentials, switch organizations or bind a conversation.
export function ConnectorDeepLink({
  connectorId,
  pendingID,
  connectionID,
}: {
  connectorId: string;
  pendingID?: string;
  connectionID?: string;
}) {
  const t = useT('connectors');
  const router = useRouter();
  const owner = useAuthStore.use.user()?.id;
  const org = useOrganizationStore.use.currentOrganization()?.id;
  const validID = (value: string) => typeof value === 'string' && /^[a-f0-9-]{36}$/.test(value);
  const invalid =
    Boolean(pendingID && connectionID) ||
    Boolean(pendingID && !validID(pendingID)) ||
    Boolean(connectionID && !validID(connectionID));
  const pendingQuery = usePendingSetup(!invalid ? pendingID : undefined);
  const catalogQuery = useConnector(connectorId, Boolean(pendingID) && !invalid);
  const [failed, setFailed] = useState(false);
  const [open, setOpen] = useState(true);
  useEffect(() => {
    setFailed(false);
    setOpen(true);
    if (!connectionID || invalid || !owner || !org) return;
    let alive = true;
    void connectorService
      .getConnection(connectionID)
      .then(result => {
        if (
          !alive ||
          useAuthStore.getState().user?.id !== owner ||
          useOrganizationStore.getState().currentOrganization?.id !== org
        )
          {return;}
        if (result.data.connector_id !== connectorId) {
          setFailed(true);
          return;
        }
        router.replace(
          `/console/connectors/connections/${encodeURIComponent(result.data.id)}?action=credentials`
        );
      })
      .catch(() => {
        if (alive) setFailed(true);
      });
    return () => {
      alive = false;
    };
  }, [connectionID, pendingID, connectorId, invalid, owner, org, router]);
  if (!pendingID && !connectionID) return null;
  const pending = pendingQuery.data?.data;
  if (
    invalid ||
    failed ||
    pendingQuery.isError ||
    catalogQuery.isError ||
    (pending && pending.connector_id !== connectorId)
  )
    {return (
      <ConnectorPageMessage
        kind="error"
        title={t('pending.invalidLink')}
        description={t('pending.closeHint')}
      />
    );}
  return pending && catalogQuery.data?.data ? (
    <ConnectorCreateDialog
      connector={catalogQuery.data.data}
      pendingSetupID={pending.id}
      open={open}
      onOpenChange={next => {
        setOpen(next);
        if (!next) router.replace(`/console/connectors/${encodeURIComponent(connectorId)}`);
      }}
    />
  ) : null;
}
