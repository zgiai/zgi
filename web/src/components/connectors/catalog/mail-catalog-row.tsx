'use client';
import { useState } from 'react';
import Link from 'next/link';
import { ChevronDown, CircleAlert, CircleCheck, KeyRound, Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ConnectorIcon } from '../connector-icon';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
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
  const canConnect = group.providers.some(mailSetupEnabled);
  const hasConnections = group.count > 0;
  const needsAttention = group.attentionCount > 0;
  const summary = hasConnections
    ? t('catalog.connectedCount', { count: group.count })
    : t('picker.states.notConnected');
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
      <div className="mt-auto flex h-9 min-w-0 items-center justify-between gap-3">
        <div
          className={cn(
            'flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground',
            needsAttention && 'text-warning'
          )}
        >
          {needsAttention ? (
            <CircleAlert aria-hidden="true" className="size-3.5 shrink-0" />
          ) : hasConnections ? (
            <CircleCheck aria-hidden="true" className="size-3.5 shrink-0 text-success" />
          ) : (
            <KeyRound aria-hidden="true" className="size-3.5 shrink-0" />
          )}
          <div className="min-w-0">
            <p className="truncate" title={summary}>
              {summary}
            </p>
            {needsAttention ? (
              <p
                className="truncate text-[11px]"
                title={t('catalog.needsAttentionCount', { count: group.attentionCount })}
              >
                {t('catalog.needsAttentionCount', { count: group.attentionCount })}
              </p>
            ) : null}
          </div>
        </div>
        <div className="flex h-9 w-36 shrink-0">
          {hasConnections ? (
            <>
              <Button
                asChild
                variant="outline"
                className={cn('h-9 min-w-0 flex-1 px-3', canConnect && 'rounded-r-none')}
              >
                <Link href="/console/connectors/groups/mail">
                  <span className="truncate">{t('catalog.manageConnections')}</span>
                </Link>
              </Button>
              {canConnect ? (
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button
                      variant="outline"
                      isIcon
                      className="h-9 w-9 rounded-l-none border-l-0"
                      aria-label={`${t('catalog.connectionActions')}: ${t('mail.title')}`}
                    >
                      <ChevronDown aria-hidden="true" className="size-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem onSelect={() => setAdding(true)}>
                      <Plus aria-hidden="true" className="size-4" />
                      {t('connections.addConnection')}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              ) : null}
            </>
          ) : (
            <Button
              className="h-9 w-full px-3"
              disabled={!canConnect}
              onClick={() => setAdding(true)}
            >
              <Plus aria-hidden="true" className="size-4 shrink-0" />
              <span className="truncate">{t('actions.connect')}</span>
            </Button>
          )}
        </div>
      </div>
      {adding && provider && (
        <ConnectorCreateDialog connector={provider} open onOpenChange={setAdding} />
      )}
    </Card>
  );
}
