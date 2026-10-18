'use client';

import { useState } from 'react';
import { Check, ChevronDown } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import type { MarketListing, MarketOffering, MarketOperation } from '@/services/types/zgi-market';
import { useMarketLabels } from './use-market-labels';
import { ZgiMarketCodeSample } from './zgi-market-code-sample';

const knownProtocols = [
  'http',
  'openai_chat',
  'openai_embeddings',
  'openai_rerank',
  'anthropic_messages',
] as const;

const methodTone: Record<string, string> = {
  GET: 'bg-success/15 text-success',
  POST: 'bg-info/10 text-info',
  PUT: 'bg-warning/15 text-warning',
  PATCH: 'bg-warning/15 text-warning',
  DELETE: 'bg-destructive/10 text-destructive',
};

function Operation({ operation, id }: { operation: MarketOperation; id: string }) {
  const { t } = useMarketLabels();
  const [open, setOpen] = useState(false);
  const described = Boolean(operation.input_schema || operation.output_schema);
  const line = (
    <>
      <span
        className={cn(
          'inline-flex w-14 shrink-0 justify-center rounded px-1.5 py-0.5 font-mono text-[11px] font-semibold',
          methodTone[operation.method] ?? 'bg-muted text-muted-foreground'
        )}
      >
        {operation.method}
      </span>
      <span className="min-w-0 break-all text-left font-mono text-sm">{operation.path}</span>
    </>
  );
  if (!described) {
    return <li className="flex items-center gap-3 px-3 py-2">{line}</li>;
  }
  return (
    <li>
      <button
        type="button"
        className="flex w-full items-center gap-3 px-3 py-2 hover:bg-accent/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
        aria-expanded={open}
        aria-controls={id}
        onClick={() => setOpen(value => !value)}
      >
        {line}
        <span className="ml-auto flex shrink-0 items-center gap-1 text-xs text-muted-foreground">
          <span className="hidden sm:inline">
            {open
              ? t('market.catalog.detail.api.hideSchema')
              : t('market.catalog.detail.api.showSchema')}
          </span>
          <ChevronDown
            className={cn('size-4 transition-transform', open && 'rotate-180')}
            aria-hidden="true"
          />
        </span>
      </button>
      {open ? (
        <div id={id} className="grid gap-3 border-t bg-muted/20 p-3 lg:grid-cols-2">
          {operation.input_schema ? (
            <ZgiMarketCodeSample
              label={t('market.catalog.inputSchema')}
              code={JSON.stringify(operation.input_schema, null, 2)}
            />
          ) : null}
          {operation.output_schema ? (
            <ZgiMarketCodeSample
              label={t('market.catalog.outputSchema')}
              code={JSON.stringify(operation.output_schema, null, 2)}
            />
          ) : null}
        </div>
      ) : null}
    </li>
  );
}

interface ZgiMarketApiSectionProps {
  listing: MarketListing;
  gatewayOrigin: string;
  isActivated: (offering: MarketOffering) => boolean;
  /** Undefined when the caller may not activate. */
  onActivate?: (offering: MarketOffering) => void;
  /** Whether the caller may activate this offering; every one when not given. */
  canActivate?: (offering: MarketOffering) => boolean;
  activating: boolean;
}

/** The offerings of an API listing: what each one is, what it costs, and what may be called. */
export function ZgiMarketApiSection({
  listing,
  gatewayOrigin,
  isActivated,
  onActivate,
  canActivate,
  activating,
}: ZgiMarketApiSectionProps) {
  const { t, meter, amount, per } = useMarketLabels();
  return (
    <ul className="flex flex-col gap-4">
      {listing.offerings.map(offering => {
        const operations = offering.operations ?? [];
        const prices = (listing.prices ?? []).filter(
          price => price.offering_slug === offering.slug
        );
        const protocol = knownProtocols.find(known => known === offering.protocol_family);
        return (
          <li key={offering.id} className="overflow-hidden rounded-lg border bg-background">
            <div className="flex flex-wrap items-start justify-between gap-3 p-4">
              <div className="min-w-0">
                <h3 className="break-all font-mono text-sm font-semibold">{offering.slug}</h3>
                <p className="mt-1 text-xs text-muted-foreground">
                  {[
                    offering.provider,
                    offering.tier,
                    protocol
                      ? t(`market.catalog.detail.api.protocols.${protocol}`)
                      : offering.protocol_family,
                  ]
                    .filter(Boolean)
                    .join(' · ')}
                </p>
              </div>
              {isActivated(offering) ? (
                <Badge variant="success" className="gap-1">
                  <Check className="size-3.5" aria-hidden="true" />
                  {t('market.catalog.activated')}
                </Badge>
              ) : onActivate && (!canActivate || canActivate(offering)) ? (
                <Button size="sm" disabled={activating} onClick={() => onActivate(offering)}>
                  {t('market.catalog.activate')}
                </Button>
              ) : null}
            </div>
            <dl className="grid gap-x-6 gap-y-2 border-t bg-muted/20 px-4 py-3 text-xs sm:grid-cols-[auto_minmax(0,1fr)]">
              <dt className="text-muted-foreground">{t('market.catalog.detail.api.baseUrl')}</dt>
              <dd className="break-all font-mono">
                {gatewayOrigin}/v1/gateway/{offering.slug}
              </dd>
              <dt className="text-muted-foreground">{t('market.catalog.detail.api.meters')}</dt>
              <dd className="flex flex-wrap gap-x-4 gap-y-1">
                {prices.length > 0
                  ? prices.map(price => (
                      <span key={price.meter}>
                        <span className="font-medium text-foreground">{amount(price)}</span>
                        <span className="text-muted-foreground"> · {per(price)}</span>
                      </span>
                    ))
                  : offering.meters.map(value => <span key={value}>{meter(value)}</span>)}
              </dd>
            </dl>
            {operations.length > 0 ? (
              <div className="border-t">
                <p className="px-4 pb-1 pt-3 text-xs font-medium text-muted-foreground">
                  {t('market.catalog.detail.api.operationCount', { count: operations.length })}
                </p>
                <ul className="divide-y">
                  {operations.map((operation, index) => (
                    <Operation
                      key={`${operation.method} ${operation.path}`}
                      operation={operation}
                      id={`zgi-market-operation-${offering.id}-${index}`}
                    />
                  ))}
                </ul>
              </div>
            ) : (
              <p className="border-t px-4 py-3 text-xs text-muted-foreground">
                {t('market.catalog.detail.api.noOperations')}
              </p>
            )}
          </li>
        );
      })}
    </ul>
  );
}
