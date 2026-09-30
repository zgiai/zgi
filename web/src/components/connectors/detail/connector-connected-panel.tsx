'use client';

import Link from 'next/link';
import { LockKeyhole } from 'lucide-react';
import { ConnectorStatusBadge } from '@/components/connectors/connector-status-badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useT } from '@/i18n';
import type { ConnectorCatalogItem } from '@/services/types/connector';

export function ConnectorConnectedPanel({ connector }: { connector: ConnectorCatalogItem }) {
  const t = useT('connectors');
  const connectionID = connector.connection_summary.primary_connection_id;
  const status = connector.connection_summary.state ?? 'unavailable';
  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <CardTitle>{t('detail.connectedTitle')}</CardTitle>
          <ConnectorStatusBadge status={status} />
        </div>
        <p className="text-sm leading-5 text-muted-foreground">
          {t('detail.connectedDescription')}
        </p>
      </CardHeader>
      <CardContent className="space-y-4">
        {connector.connection_summary.external_display_name ? (
          <p className="rounded-lg border bg-muted/20 px-3 py-2 text-sm font-medium">
            {t('catalog.connectedAs', {
              name: connector.connection_summary.external_display_name,
            })}
          </p>
        ) : null}
        {connectionID ? (
          <Button asChild className="w-full">
            <Link href={`/console/connectors/connections/${encodeURIComponent(connectionID)}`}>
              {t('detail.manageConnection')}
            </Link>
          </Button>
        ) : null}
        <p className="flex items-start gap-2 border-t pt-4 text-xs leading-5 text-muted-foreground">
          <LockKeyhole aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
          <span>{t('setup.securityNotice')}</span>
        </p>
      </CardContent>
    </Card>
  );
}
