'use client';

import Link from 'next/link';
import { Trash2 } from 'lucide-react';
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
import { useLocale } from '@/hooks/use-locale';
import {
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
          listing => listing.listing_id === item.listing_id
        );
        // The stored title is in one language; the listing itself has both.
        const name = marketText(candidate?.title, locale) || item.title || item.slug;
        const usable = candidate ? usableMarketConnections(candidate, selection.connections) : [];
        const needsConnection = (candidate?.required_connectors.length ?? 0) > 0;
        return (
          <li key={item.listing_id} className="rounded-md border bg-background p-3">
            <div className="flex items-start gap-3">
              <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md border bg-muted text-primary">
                <ConnectorIcon
                  connectorId={`zgi-market:${item.slug}`}
                  name={name}
                  classification="connector"
                  className="size-8 border-0 shadow-none"
                />
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <Link
                    href={`/dashboard/market/catalog/mcp/${item.slug}`}
                    className="min-w-0 truncate text-sm font-medium hover:underline"
                  >
                    {name}
                  </Link>
                </div>
                <div className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                  {t('market.tools.rowDescription', {
                    kind: t('market.catalog.kinds.mcp'),
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
                    () => selection.remove(item.listing_id),
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
              {item.tools.slice(0, 6).map(tool => (
                <Badge
                  key={tool.name}
                  variant={tool.risk_level === 'read' ? 'subtle' : 'warning'}
                  className="text-[10px]"
                  title={
                    marketText(
                      candidate?.tools.find(known => known.name === tool.name)?.description,
                      locale
                    ) || tool.name
                  }
                >
                  <span className="font-mono">{tool.name}</span>
                  {' · '}
                  {t(`market.catalog.risk.${tool.risk_level}`)}
                </Badge>
              ))}
              {item.tools.length > 6 ? (
                <Badge variant="subtle" className="text-[10px]">
                  +{item.tools.length - 6}
                </Badge>
              ) : null}
            </div>
            {needsConnection ? (
              <div className="mt-3 flex flex-wrap items-center gap-2">
                <span className="text-xs font-medium text-muted-foreground">
                  {t('market.tools.connection')}
                </span>
                {usable.length === 0 ? (
                  <Link
                    href={`/dashboard/market/catalog/mcp/${item.slug}`}
                    className="text-xs text-primary hover:underline"
                  >
                    {t('market.tools.noConnection')}
                  </Link>
                ) : (
                  <Select
                    disabled={readOnly || selection.isSaving}
                    value={item.connection_id ? String(item.connection_id) : ''}
                    onValueChange={value =>
                      void run(() => selection.setConnection(item.listing_id, Number(value)))
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
