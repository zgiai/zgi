'use client';

import type { ReactNode } from 'react';
import { ExternalLink } from 'lucide-react';
import type { MarketListing } from '@/services/types/zgi-market';
import {
  chargedByUsage,
  lowestPrice,
  marketDate,
  marketExternalUrl,
  marketUnitName,
} from './market-format';
import { useMarketLabels } from './use-market-labels';

const reviewStatuses = ['unreviewed', 'pending_review', 'reviewed', 'verified'] as const;
const connectorAuthTypes = ['oauth2', 'api_key', 'none'] as const;

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[5.5rem_minmax(0,1fr)] gap-x-3 py-2 text-sm">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="min-w-0 break-words">{children}</dd>
    </div>
  );
}

function External({ href }: { href: string }) {
  let label = href;
  try {
    const url = new URL(href);
    label = `${url.host}${url.pathname === '/' ? '' : url.pathname}`;
  } catch {
    // The caller only passes an address that parsed; keep it as it is otherwise.
  }
  return (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      className="inline-flex max-w-full items-center gap-1 text-primary hover:underline"
    >
      <span className="min-w-0 truncate">{label}</span>
      <ExternalLink className="size-3.5 shrink-0" aria-hidden="true" />
    </a>
  );
}

interface ZgiMarketListingPanelProps {
  listing: MarketListing;
  /** The primary action of the listing with its state, or the note that replaces it. */
  action: ReactNode;
  /** Shown under the action: required connectors, a hint. */
  children?: ReactNode;
}

/** The side panel of a listing: what it costs, what the organization can do now, and its facts. */
export function ZgiMarketListingPanel({ listing, action, children }: ZgiMarketListingPanelProps) {
  const { t, text, meter, amount, per, cap, locale } = useMarketLabels();
  const price = lowestPrice(listing);
  const several = (listing.prices ?? []).length > 1;
  // What one call can cost at most, when the listing is charged by what a call used.
  const atMost = chargedByUsage(listing.prices ?? []) ? cap(listing) : '';
  const providers = [
    ...new Set(listing.offerings.map(offering => offering.provider).filter(Boolean)),
  ];
  const meters = [
    ...new Set([
      ...listing.offerings.flatMap(offering => offering.meters),
      ...(listing.prices ?? []).map(item => item.meter),
    ]),
  ];
  const published = marketDate(
    listing.current_version?.published_at ?? listing.published_at,
    locale
  );
  const origin = marketExternalUrl(listing.detail.origin_url);
  const repository = marketExternalUrl(listing.detail.source_repository);
  const review = reviewStatuses.find(status => status === listing.detail.review_status);
  const authType = connectorAuthTypes.find(type => type === listing.connector?.auth_type);
  const author = listing.plugin?.manifest?.author;
  const categories = [listing.primary_category, ...(listing.secondary_categories ?? [])].filter(
    (category): category is NonNullable<typeof category> => Boolean(category)
  );

  return (
    <aside
      aria-label={t('market.catalog.detail.panel.facts')}
      className="flex flex-col gap-4 lg:sticky lg:top-0"
    >
      <div className="flex flex-col gap-4 rounded-xl border bg-background p-5 shadow-sm">
        {listing.kind === 'connector' ? null : (
          <div>
            {price ? (
              <p className="flex flex-wrap items-baseline gap-x-1.5">
                <span className="text-2xl font-semibold tabular-nums">{amount(price)}</span>
                {several && price.unit_price_micros > 0 ? (
                  <span className="text-sm text-muted-foreground">
                    {t('market.catalog.detail.panel.from')}
                  </span>
                ) : null}
              </p>
            ) : (
              <p className="text-sm text-muted-foreground">
                {t('market.catalog.detail.panel.priceUnknown')}
              </p>
            )}
            {price ? <p className="mt-0.5 text-xs text-muted-foreground">{per(price)}</p> : null}
            {price && atMost ? (
              <p className="mt-0.5 text-xs text-muted-foreground" data-market-price-cap="">
                {atMost}
              </p>
            ) : null}
          </div>
        )}
        {action}
        {children}
      </div>

      <div className="rounded-xl border bg-background px-5 py-3">
        <h2 className="pb-1 pt-2 text-sm font-semibold">
          {t('market.catalog.detail.panel.facts')}
        </h2>
        <dl className="divide-y">
          <Fact label={t('market.catalog.detail.panel.kind')}>
            {t(`market.catalog.kinds.${listing.kind}`)}
          </Fact>
          {listing.current_version?.label ? (
            <Fact label={t('market.catalog.detail.panel.version')}>
              <span className="font-mono text-xs">{listing.current_version.label}</span>
            </Fact>
          ) : null}
          {published ? (
            <Fact label={t('market.catalog.detail.panel.published')}>{published}</Fact>
          ) : null}
          {categories.length > 0 ? (
            <Fact label={t('market.catalog.detail.panel.category')}>
              {categories.map(category => text(category.name) || category.slug).join(' · ')}
            </Fact>
          ) : null}
          {providers.length > 0 ? (
            <Fact label={t('market.catalog.detail.panel.provider')}>{providers.join(' · ')}</Fact>
          ) : null}
          {author ? <Fact label={t('market.catalog.detail.panel.author')}>{author}</Fact> : null}
          {authType ? (
            <Fact label={t('market.catalog.detail.connector.authTitle')}>
              {t(`market.catalog.detail.connector.auth.${authType}`)}
            </Fact>
          ) : null}
          {meters.length > 0 ? (
            <Fact label={t('market.catalog.detail.panel.meters')}>
              {meters
                .map(name =>
                  name === 'unit' ? marketUnitName(listing) || meter(name) : meter(name)
                )
                .join(' · ')}
            </Fact>
          ) : null}
          {listing.detail.license ? (
            <Fact label={t('market.catalog.detail.panel.license')}>{listing.detail.license}</Fact>
          ) : null}
          {origin ? (
            <Fact label={t('market.catalog.detail.panel.source')}>
              <External href={origin} />
            </Fact>
          ) : null}
          {repository && repository !== origin ? (
            <Fact label={t('market.catalog.detail.panel.repository')}>
              <External href={repository} />
            </Fact>
          ) : null}
          {review ? (
            <Fact label={t('market.catalog.detail.panel.review')}>
              {t(`market.catalog.detail.panel.reviewStatus.${review}`)}
            </Fact>
          ) : null}
          <Fact label={t('market.catalog.detail.panel.slug')}>
            <span className="break-all font-mono text-xs">{listing.slug}</span>
          </Fact>
        </dl>
      </div>
    </aside>
  );
}
