'use client';
import { useState } from 'react';
import Link from 'next/link';
import { Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { ConnectorIcon } from '../connector-icon';
import { useT } from '@/i18n';
import type { MailDisplayGroup } from '../mail-display-model';
import { selectMailProvider } from '../mail-display-model';
import { mailSetupEnabled } from '../setup/mail-setup-state';
import { ConnectorCreateDialog } from '../setup/connector-create-dialog';
import { ConnectorTaxonomyBadges } from '../connector-taxonomy-badges';

export function MailCatalogRow({
  group,
  preferredProvider,
}: {
  group: MailDisplayGroup;
  preferredProvider?: string;
}) {
  const t = useT('connectors');
  const [adding, setAdding] = useState(false);
  const provider = selectMailProvider(group.providers, preferredProvider);
  return (
    <Card
      role="article"
      className="flex h-full min-w-0 flex-col gap-4 bg-background p-4 shadow-xs sm:p-5"
    >
      <div className="flex min-w-0 items-start gap-4">
        <ConnectorIcon connectorId="mail" name={t('mail.title')} classification="connector" />
        <div className="min-w-0 flex-1">
          <h3 className="truncate text-sm font-semibold">
            <Link href="/console/connectors/groups/mail">{t('mail.title')}</Link>
          </h3>
          <p className="mt-1 h-10 line-clamp-2 text-sm leading-5 text-muted-foreground">
            {t('mail.catalogDescription')}
          </p>
          <ConnectorTaxonomyBadges
            item={group.providers[0] ?? {}}
            className="mt-2 flex min-w-0 flex-wrap gap-1.5"
          />
        </div>
      </div>
      <div className="mt-auto flex flex-wrap items-center justify-between gap-2">
        <div className="text-xs text-muted-foreground">
          <p>{t('connections.count', { count: group.count })}</p>
          {group.attentionCount > 0 && (
            <p className="text-warning">
              {t('catalog.needsAttentionCount', { count: group.attentionCount })}
            </p>
          )}
        </div>
        <div className="flex gap-2">
          {group.count > 0 && (
            <Button asChild size="sm" variant="outline">
              <Link href="/console/connectors/groups/mail">{t('catalog.manageConnections')}</Link>
            </Button>
          )}
          <Button
            size="sm"
            disabled={!group.providers.some(mailSetupEnabled)}
            onClick={() => setAdding(true)}
          >
            <Plus aria-hidden="true" className="size-4" />
            {t(group.count > 0 ? 'connections.addConnection' : 'actions.connect')}
          </Button>
        </div>
      </div>
      {adding && provider && (
        <ConnectorCreateDialog connector={provider} open onOpenChange={setAdding} />
      )}
    </Card>
  );
}
