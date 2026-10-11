'use client';

import { use } from 'react';
import Link from 'next/link';
import { ArrowLeft, Check } from 'lucide-react';
import { toast } from 'sonner';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import MarketEmptyState from '@/components/market/market-empty-state';
import { marketAmount, marketText } from '@/components/zgi-market/market-format';
import { useLocale } from '@/hooks/use-locale';
import {
  useActivateZgiMarketListing,
  useZgiMarketListing,
  useZgiMarketSubscriptions,
} from '@/hooks/use-zgi-market';
import { useT } from '@/i18n';
import {
  marketListingKinds,
  type MarketListingKind,
  type MarketOffering,
} from '@/services/types/zgi-market';

interface PageProps {
  params: Promise<{ kind: string; slug: string }>;
}

// The meters the market names today. A meter added later is shown by its identifier.
const knownMeters = [
  'request',
  'token_input',
  'token_output',
  'page',
  'image',
  'second',
  'character',
  'tool_call',
  'install',
] as const;

function Schema({ label, schema }: { label: string; schema: Record<string, unknown> }) {
  return (
    <figure className="overflow-hidden rounded-md border bg-muted/40">
      <figcaption className="border-b bg-muted/60 px-3 py-1.5 text-xs font-medium text-muted-foreground">
        {label}
      </figcaption>
      <pre className="max-h-72 overflow-auto p-3 text-xs leading-relaxed">
        <code className="font-mono">{JSON.stringify(schema, null, 2)}</code>
      </pre>
    </figure>
  );
}

export default function ZgiMarketListingPage({ params }: PageProps) {
  const { kind: rawKind, slug } = use(params);
  const kind = (marketListingKinds as readonly string[]).includes(rawKind)
    ? (rawKind as MarketListingKind)
    : undefined;
  const t = useT();
  const { locale } = useLocale();
  const listing = useZgiMarketListing(kind, slug);
  const subscriptions = useZgiMarketSubscriptions();
  const activate = useActivateZgiMarketListing();

  const back = (
    <Link
      href="/dashboard/market/catalog"
      className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
    >
      <ArrowLeft className="size-4" aria-hidden="true" />
      {t('market.catalog.back')}
    </Link>
  );

  if (!kind || listing.isError) {
    return (
      <div className="flex h-full flex-col overflow-y-auto px-6 py-5">
        {back}
        <MarketEmptyState
          title={t('market.catalog.notFound')}
          actionLabel={kind ? t('market.catalog.retry') : undefined}
          onAction={kind ? () => void listing.refetch() : undefined}
        />
      </div>
    );
  }
  if (listing.isPending) {
    return (
      <div className="flex h-full flex-col gap-4 overflow-y-auto px-6 py-5">
        {back}
        <Skeleton className="h-8 w-72" />
        <Skeleton className="h-24 w-full max-w-3xl" />
        <Skeleton className="h-48 w-full max-w-3xl" />
      </div>
    );
  }

  const data = listing.data;
  const title = marketText(data.card.title, locale) || data.slug;
  const body = marketText(data.detail.body, locale);
  const active = (subscriptions.data ?? []).filter(
    subscription => subscription.status === 'active' && subscription.listing_id === data.id
  );
  const isActivated = (offering?: MarketOffering) =>
    active.some(subscription => !offering || subscription.offering_id === offering.id);
  const run = (offering?: MarketOffering) =>
    activate.mutate(
      { listingId: data.id, ...(offering ? { offeringId: offering.id } : {}) },
      {
        onSuccess: () => toast.success(t('market.catalog.activateDone')),
        onError: () => toast.error(t('market.catalog.activateFailed')),
      }
    );
  const pricesOf = (offeringId: number) =>
    (data.prices ?? []).filter(price => price.offering_id === offeringId);
  const listingPrices = (data.prices ?? []).filter(price => !price.offering_id);
  const priceText = (price: (typeof listingPrices)[number]) => {
    if (price.unit_price_micros === 0) {
      return t('market.catalog.free');
    }
    const { amount, unit } = marketAmount(price);
    const known = knownMeters.find(meter => meter === price.meter);
    return t(`market.catalog.price.${unit}`, {
      amount,
      quantity: price.per_quantity,
      meter: known ? t(`market.catalog.meters.${known}`) : price.meter,
    });
  };
  // A connector is connected, not activated, and an API is activated per offering.
  const activatesWhole = data.kind !== 'api' && data.kind !== 'connector';

  return (
    <div className="flex h-full flex-col overflow-y-auto px-6 py-5">
      {back}
      <header className="mt-4 flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="break-words text-2xl font-semibold">{title}</h1>
            <Badge variant="secondary">{t(`market.catalog.kinds.${data.kind}`)}</Badge>
            {data.current_version?.label ? (
              <Badge variant="outline">{data.current_version.label}</Badge>
            ) : null}
          </div>
          <p className="mt-2 max-w-3xl text-sm text-muted-foreground">
            {marketText(data.card.description, locale)}
          </p>
          {data.tags.length > 0 ? (
            <div className="mt-3 flex flex-wrap gap-1.5">
              {data.tags.map(tag => (
                <Badge key={tag} variant="subtle">
                  {tag}
                </Badge>
              ))}
            </div>
          ) : null}
        </div>
        {activatesWhole ? (
          <div className="flex flex-col items-end gap-1">
            {isActivated() ? (
              <Badge variant="success" className="gap-1">
                <Check className="size-3.5" aria-hidden="true" />
                {t('market.catalog.activated')}
              </Badge>
            ) : (
              <Button disabled={activate.isPending} onClick={() => run()}>
                {t('market.catalog.activate')}
              </Button>
            )}
            {listingPrices.map(price => (
              <span key={price.meter} className="text-xs text-muted-foreground">
                {priceText(price)}
              </span>
            ))}
          </div>
        ) : null}
      </header>

      {data.offerings.length > 0 ? (
        <section className="mt-8 max-w-4xl" aria-labelledby="zgi-market-offerings">
          <h2 id="zgi-market-offerings" className="text-base font-semibold">
            {t('market.catalog.offerings')}
          </h2>
          <ul className="mt-3 flex flex-col gap-4">
            {data.offerings.map(offering => (
              <li key={offering.id} className="rounded-lg border p-4">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="break-all font-mono text-sm font-medium">{offering.slug}</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {[offering.provider, offering.tier, offering.protocol_family]
                        .filter(Boolean)
                        .join(' · ')}
                    </p>
                    <ul className="mt-2 flex flex-col gap-0.5 text-xs text-muted-foreground">
                      {pricesOf(offering.id).map(price => (
                        <li key={price.meter}>{priceText(price)}</li>
                      ))}
                    </ul>
                  </div>
                  {data.kind === 'api' ? (
                    isActivated(offering) ? (
                      <Badge variant="success" className="gap-1">
                        <Check className="size-3.5" aria-hidden="true" />
                        {t('market.catalog.activated')}
                      </Badge>
                    ) : (
                      <Button size="sm" disabled={activate.isPending} onClick={() => run(offering)}>
                        {t('market.catalog.activate')}
                      </Button>
                    )
                  ) : null}
                </div>
                {(offering.operations ?? []).length > 0 ? (
                  <div className="mt-4 flex flex-col gap-3">
                    {(offering.operations ?? []).map(operation => (
                      <div
                        key={`${operation.method} ${operation.path}`}
                        className="flex flex-col gap-2"
                      >
                        <p className="font-mono text-xs">
                          <span className="font-semibold">{operation.method}</span> {operation.path}
                        </p>
                        {operation.input_schema ? (
                          <Schema
                            label={t('market.catalog.inputSchema')}
                            schema={operation.input_schema}
                          />
                        ) : null}
                        {operation.output_schema ? (
                          <Schema
                            label={t('market.catalog.outputSchema')}
                            schema={operation.output_schema}
                          />
                        ) : null}
                      </div>
                    ))}
                  </div>
                ) : null}
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {data.mcp && data.mcp.tools.length > 0 ? (
        <section className="mt-8 max-w-4xl" aria-labelledby="zgi-market-tools">
          <h2 id="zgi-market-tools" className="text-base font-semibold">
            {t('market.catalog.tools')}
          </h2>
          <ul className="mt-3 divide-y rounded-lg border">
            {data.mcp.tools.map(tool => (
              <li key={tool.name} className="flex flex-wrap items-start justify-between gap-3 p-3">
                <div className="min-w-0">
                  <p className="font-mono text-sm">{tool.name}</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {marketText(tool.description, locale)}
                  </p>
                </div>
                <Badge variant={tool.risk_level === 'read' ? 'subtle' : 'warning'}>
                  {t(`market.catalog.risk.${tool.risk_level}`)}
                </Badge>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {body ? (
        <section className="mt-8 max-w-4xl" aria-labelledby="zgi-market-about">
          <h2 id="zgi-market-about" className="text-base font-semibold">
            {t('market.catalog.about')}
          </h2>
          {/* The body is written by whoever published the listing: raw HTML is never rendered. */}
          <MarkdownViewer content={body} allowRawHtml={false} className="mt-3" />
        </section>
      ) : null}
    </div>
  );
}
