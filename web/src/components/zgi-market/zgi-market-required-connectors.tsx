'use client';

import { useEffect, useState } from 'react';
import { Check, Link2 } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { marketText } from '@/components/zgi-market/market-format';
import { useLocale } from '@/hooks/use-locale';
import { useZgiMarketConnections } from '@/hooks/use-zgi-market';
import { useT } from '@/i18n';
import type { MarketRequiredConnector } from '@/services/types/zgi-market';
import { zgiMarketService } from '@/services/zgi-market.service';

interface ZgiMarketRequiredConnectorsProps {
  kind: string;
  slug: string;
  connectors: MarketRequiredConnector[];
}

/**
 * The connectors an MCP listing calls the vendor through, and the OAuth authorization that
 * connects the organization's own account. The vendor sends the user back to this listing page.
 */
export function ZgiMarketRequiredConnectors({ kind, slug, connectors }: ZgiMarketRequiredConnectorsProps) {
  const t = useT();
  const { locale } = useLocale();
  const connections = useZgiMarketConnections(connectors.length > 0);
  const [pending, setPending] = useState<string | null>(null);

  useEffect(() => {
    const query = new URLSearchParams(window.location.search);
    const code = query.get('error_code');
    if (query.has('connected') || code) {
      if (code) {
        toast.error(t('market.catalog.connectors.returnFailed', { code }));
      } else {
        toast.success(t('market.catalog.connectors.returned'));
      }
      window.history.replaceState(null, '', window.location.pathname);
    }
  }, [t]);

  if (connectors.length === 0) {
    return null;
  }

  const connect = async (connector: MarketRequiredConnector) => {
    setPending(connector.slug);
    try {
      const returnUrl = `${window.location.origin}/dashboard/market/catalog/${kind}/${slug}?connected=1`;
      const name = marketText(connector.title, locale) || connector.slug;
      const started = await zgiMarketService.connect(connector.slug, name, returnUrl);
      if (!started.authorize_url) {
        await connections.refetch();
        setPending(null);
        return;
      }
      window.location.assign(started.authorize_url);
    } catch {
      toast.error(t('market.catalog.connectors.connectFailed'));
      setPending(null);
    }
  };

  return (
    <section className="mt-8 max-w-4xl" aria-labelledby="zgi-market-connectors">
      <h2 id="zgi-market-connectors" className="text-base font-semibold">
        {t('market.catalog.connectors.title')}
      </h2>
      <p className="mt-1 text-xs text-muted-foreground">{t('market.catalog.connectors.description')}</p>
      <ul className="mt-3 divide-y rounded-lg border">
        {connectors.map(connector => {
          const connected = (connections.data ?? []).some(
            connection => connection.connector_slug === connector.slug && connection.status === 'connected'
          );
          return (
            <li key={connector.slug} className="flex flex-wrap items-center justify-between gap-3 p-3">
              <span className="flex items-center gap-2 text-sm">
                <Link2 className="size-4 text-muted-foreground" aria-hidden="true" />
                {marketText(connector.title, locale) || connector.slug}
              </span>
              {connected ? (
                <Badge variant="success" className="gap-1">
                  <Check className="size-3.5" aria-hidden="true" />
                  {t('market.catalog.connectors.connected')}
                </Badge>
              ) : connector.auth_type === 'oauth2' || connector.auth_type === 'none' ? (
                <Button size="sm" disabled={pending !== null} onClick={() => void connect(connector)}>
                  {pending === connector.slug
                    ? t('market.catalog.connectors.connecting')
                    : t('market.catalog.connectors.connect')}
                </Button>
              ) : (
                <span className="text-xs text-muted-foreground">{t('market.catalog.connectors.keyRequired')}</span>
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
