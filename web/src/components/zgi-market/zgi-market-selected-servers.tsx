'use client';

import { useState } from 'react';
import Link from 'next/link';
import { SlidersHorizontal, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import { ConnectorIcon } from '@/components/connectors/connector-icon';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { marketText } from '@/components/zgi-market/market-format';
import { ZgiMarketCallPrice } from '@/components/zgi-market/zgi-market-call-price';
import { ZgiMarketOperationsDialog } from '@/components/zgi-market/zgi-market-operations-dialog';
import { useLocale } from '@/hooks/use-locale';
import {
  bindableOperations,
  boundToolChips,
  marketItemHref,
  marketItemKind,
  usableMarketConnections,
  useZgiMarketSubjectSelection,
} from '@/hooks/use-zgi-market-subject-tools';
import { useT } from '@/i18n';
import type { MarketToolSubjectType } from '@/services/types/zgi-market';
import { getErrorMessage } from '@/utils/error-notifications';

interface ZgiMarketSelectedServersProps {
  subjectType: MarketToolSubjectType;
  subjectId: string;
  readOnly?: boolean;
}

/** How many ZGI Market servers a subject has, for the count of its connector section. */
export function useZgiMarketSelectedCount(subjectType: MarketToolSubjectType, subjectId: string) {
  return useZgiMarketSubjectSelection(subjectType, subjectId).items.length;
}

/**
 * The ZGI Market servers bound to an agent or a digital human, listed with its connectors. Each
 * can be removed, and one that acts with the organization's connection names the connection.
 */
export function ZgiMarketSelectedServers({
  subjectType,
  subjectId,
  readOnly = false,
}: ZgiMarketSelectedServersProps) {
  const t = useT();
  const { locale } = useLocale();
  const selection = useZgiMarketSubjectSelection(subjectType, subjectId);
  // The bound API offering whose operations are being changed, by its activation.
  const [choosing, setChoosing] = useState<number | null>(null);

  if (selection.items.length === 0) return null;

  const run = async (action: () => Promise<unknown>, done?: string) => {
    try {
      await action();
      if (done) toast.success(done);
    } catch (error) {
      toast.error(t('market.tools.saveFailed', { message: getErrorMessage(error) }));
    }
  };

  return (
    <ul className="space-y-2" data-testid="zgi-market-selected-servers">
      {selection.items.map(item => {
        const candidate = selection.listings.find(
          listing => listing.subscription_id === item.subscription_id
        );
        const kind = marketItemKind(item);
        const href = marketItemHref({ kind, slug: item.slug });
        // The stored title is in one language; the listing itself has both.
        const title = marketText(candidate?.title, locale) || item.title || item.slug;
        const name =
          kind === 'api' && item.offering_slug ? `${title} · ${item.offering_slug}` : title;
        const usable = candidate ? usableMarketConnections(candidate, selection.connections) : [];
        const needsConnection = (candidate?.required_connectors.length ?? 0) > 0;
        const total = candidate ? bindableOperations(candidate).length : 0;
        const choosable = kind === 'api' && Boolean(candidate) && total > 1;
        return (
          <li
            key={item.subscription_id}
            data-market-kind={kind}
            className="rounded-md border bg-background p-3"
          >
            <div className="flex items-start gap-3">
              <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md border bg-muted text-primary">
                <ConnectorIcon
                  connectorId={`zgi-market:${item.offering_slug ?? item.slug}`}
                  name={name}
                  classification="connector"
                  className="size-8 border-0 shadow-none"
                />
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <Link
                    href={href}
                    className="min-w-0 truncate text-sm font-medium hover:underline"
                  >
                    {name}
                  </Link>
                </div>
                <div className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                  {t('market.tools.rowDescription', {
                    kind: t(`market.catalog.kinds.${kind}`),
                    count: item.tools.length,
                  })}
                </div>
              </div>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                isIcon
                className="size-8 shrink-0 text-muted-foreground hover:text-destructive"
                aria-label={t('market.tools.remove', { name })}
                disabled={readOnly || selection.isSaving}
                onClick={() =>
                  void run(
                    () => selection.remove(item.subscription_id),
                    t('market.tools.removedToast', { name })
                  )
                }
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
            <div className="mt-3 flex flex-wrap gap-1">
              <Badge variant="outline" className="text-[10px]">
                {t('market.tools.fromMarket')}
              </Badge>
              {kind === 'api' && candidate ? (
                <Badge variant="outline" className="text-[10px]">
                  {item.all_operations
                    ? t('market.tools.operations.summaryAll', { count: item.tools.length })
                    : t('market.tools.operations.summarySome', {
                        count: (item.operations ?? []).length,
                        total,
                      })}
                </Badge>
              ) : null}
              {boundToolChips(item, candidate, locale)
                .slice(0, 6)
                .map(tool => (
                  <Badge
                    key={tool.key}
                    variant={tool.risk === 'read' ? 'subtle' : 'warning'}
                    className="text-[10px]"
                    title={tool.hint}
                  >
                    <span className="font-mono">{tool.label}</span>
                    {' · '}
                    {t(`market.catalog.risk.${tool.risk}`)}
                  </Badge>
                ))}
              {item.tools.length > 6 ? (
                <Badge variant="subtle" className="text-[10px]">
                  +{item.tools.length - 6}
                </Badge>
              ) : null}
            </div>
            <div className="mt-2 flex flex-wrap items-center gap-2">
              <ZgiMarketCallPrice
                listing={{ kind, slug: item.slug, offering_slug: item.offering_slug }}
              />
              {choosable && candidate ? (
                <>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-7 gap-1 px-2 text-xs"
                    disabled={readOnly || selection.isSaving}
                    aria-label={`${t('market.tools.operations.change')}: ${name}`}
                    onClick={() => setChoosing(item.subscription_id)}
                  >
                    <SlidersHorizontal className="size-3.5" aria-hidden="true" />
                    {t('market.tools.operations.change')}
                  </Button>
                  <ZgiMarketOperationsDialog
                    open={choosing === item.subscription_id}
                    onOpenChange={next => setChoosing(next ? item.subscription_id : null)}
                    listing={candidate}
                    name={name}
                    selected={
                      item.all_operations
                        ? undefined
                        : (item.operations ?? []).map(operation => operation.key)
                    }
                    saving={selection.isSaving}
                    confirmLabel={t('market.tools.operations.save')}
                    onConfirm={operations =>
                      void run(
                        async () => {
                          await selection.setOperations(item.subscription_id, operations);
                          setChoosing(null);
                        },
                        t('market.tools.operations.savedToast', { name })
                      )
                    }
                  />
                </>
              ) : null}
            </div>
            {needsConnection ? (
              <div className="mt-3 flex flex-wrap items-center gap-2">
                <span className="text-xs font-medium text-muted-foreground">
                  {t('market.tools.connection')}
                </span>
                {usable.length === 0 ? (
                  <Link href={href} className="text-xs text-primary hover:underline">
                    {t('market.tools.noConnection')}
                  </Link>
                ) : (
                  <Select
                    disabled={readOnly || selection.isSaving}
                    value={item.connection_id ? String(item.connection_id) : ''}
                    onValueChange={value =>
                      void run(() => selection.setConnection(item.subscription_id, Number(value)))
                    }
                  >
                    <SelectTrigger className="h-8 w-auto min-w-36 text-xs">
                      <SelectValue placeholder={t('market.tools.connectionPlaceholder')} />
                    </SelectTrigger>
                    <SelectContent>
                      {usable.map(connection => (
                        <SelectItem key={connection.id} value={String(connection.id)}>
                          {connection.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              </div>
            ) : null}
          </li>
        );
      })}
    </ul>
  );
}
