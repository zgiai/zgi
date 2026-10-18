'use client';

import type { ReactNode } from 'react';
import Link from 'next/link';
import { ChevronRight } from 'lucide-react';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { Badge } from '@/components/ui/badge';
import { useZgiMarketListings } from '@/hooks/use-zgi-market';
import type {
  MarketListing,
  MarketListingKind,
  MarketText,
  MarketTool,
} from '@/services/types/zgi-market';
import { useMarketLabels } from './use-market-labels';
import ZgiMarketCard from './zgi-market-card';
import { ZgiMarketSkillContents } from './zgi-market-skill-contents';

function Section({
  id,
  title,
  aside,
  children,
}: {
  id: string;
  title: string;
  aside?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section aria-labelledby={id} className="flex flex-col gap-3">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 id={id} className="text-base font-semibold">
          {title}
        </h2>
        {aside ? <span className="text-xs text-muted-foreground">{aside}</span> : null}
      </div>
      {children}
    </section>
  );
}

/** The tools of an MCP server, with what each may do. */
export function ZgiMarketToolList({ tools }: { tools: MarketTool[] }) {
  const { t, text } = useMarketLabels();
  return (
    <ul className="divide-y overflow-hidden rounded-lg border bg-background">
      {tools.map(tool => (
        <li key={tool.name} className="flex flex-wrap items-start justify-between gap-3 px-4 py-3">
          <div className="min-w-0">
            <p className="break-all font-mono text-sm font-medium">{tool.name}</p>
            <p className="mt-1 text-sm text-muted-foreground">{text(tool.description)}</p>
          </div>
          <Badge variant={tool.risk_level === 'read' ? 'subtle' : 'warning'}>
            {t(`market.catalog.risk.${tool.risk_level}`)}
          </Badge>
        </li>
      ))}
    </ul>
  );
}

function ListingLink({
  kind,
  slug,
  title,
}: {
  kind: MarketListingKind;
  slug: string;
  title?: MarketText;
}) {
  const { t, text } = useMarketLabels();
  return (
    <Link
      href={`/console/market/catalog/${kind}/${slug}`}
      className="flex items-center justify-between gap-3 px-4 py-3 hover:bg-accent/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
    >
      <span className="flex min-w-0 items-center gap-2">
        <Badge variant="secondary">{t(`market.catalog.kinds.${kind}`)}</Badge>
        <span className="min-w-0 truncate text-sm font-medium">{text(title) || slug}</span>
      </span>
      <ChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
    </Link>
  );
}

const pluginComponents = ['skills', 'mcp_servers', 'commands', 'agents', 'hooks'] as const;

function PluginSummary({ listing }: { listing: MarketListing }) {
  const { t } = useMarketLabels();
  const plugin = listing.plugin;
  if (!plugin) {
    return null;
  }
  const counted = pluginComponents.filter(name => (plugin.components?.[name] ?? 0) > 0);
  const members = (plugin.members ?? []).filter(member => member.slug);
  return (
    <Section id="zgi-market-plugin" title={t('market.catalog.detail.plugin.title')}>
      {counted.length > 0 ? (
        <dl className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
          {counted.map(name => (
            <div key={name} className="rounded-lg border bg-background px-4 py-3">
              <dt className="text-xs text-muted-foreground">
                {t(`market.catalog.detail.plugin.components.${name}`)}
              </dt>
              <dd className="mt-1 text-xl font-semibold tabular-nums">
                {plugin.components?.[name]}
              </dd>
            </div>
          ))}
        </dl>
      ) : null}
      {members.length > 0 ? (
        <div className="flex flex-col gap-2">
          <h3 className="text-sm font-medium text-muted-foreground">
            {t('market.catalog.detail.plugin.members')}
          </h3>
          <ul className="divide-y overflow-hidden rounded-lg border bg-background">
            {members.map(member => (
              <li key={`${member.kind}:${member.slug}`}>
                <ListingLink kind={member.kind} slug={member.slug} title={member.title} />
              </li>
            ))}
          </ul>
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">
          {t('market.catalog.detail.plugin.noMembers')}
        </p>
      )}
      {(plugin.requirements ?? []).length > 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('market.catalog.detail.plugin.requirements')}: {(plugin.requirements ?? []).join(', ')}
        </p>
      ) : null}
    </Section>
  );
}

function ConnectorSummary({ listing }: { listing: MarketListing }) {
  const { t } = useMarketLabels();
  const connector = listing.connector;
  if (!connector) {
    return null;
  }
  const usedBy = connector.used_by ?? [];
  return (
    <Section id="zgi-market-connector" title={t('market.catalog.detail.connector.usedBy')}>
      {usedBy.length > 0 ? (
        <ul className="divide-y overflow-hidden rounded-lg border bg-background">
          {usedBy.map(user => (
            <li key={`${user.kind}:${user.slug}`}>
              <ListingLink kind={user.kind} slug={user.slug} title={user.title} />
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-sm text-muted-foreground">
          {t('market.catalog.detail.connector.noUsedBy')}
        </p>
      )}
    </Section>
  );
}

function Related({ listing, activatedIds }: { listing: MarketListing; activatedIds: Set<number> }) {
  const { t, locale } = useMarketLabels();
  const category = listing.primary_category?.slug;
  const related = useZgiMarketListings({ category, page: 1, per_page: 4 }, Boolean(category));
  const others = (related.data?.data ?? []).filter(item => item.id !== listing.id).slice(0, 3);
  if (others.length === 0) {
    return null;
  }
  return (
    <Section id="zgi-market-related" title={t('market.catalog.detail.related.title')}>
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {others.map(item => (
          <ZgiMarketCard
            key={item.id}
            listing={item}
            locale={locale}
            activated={activatedIds.has(item.id)}
          />
        ))}
      </div>
    </Section>
  );
}

interface ZgiMarketOverviewProps {
  listing: MarketListing;
  activatedIds: Set<number>;
}

/** What the listing is: the publisher's own words, then what it contains. */
export function ZgiMarketOverview({ listing, activatedIds }: ZgiMarketOverviewProps) {
  const { t, text } = useMarketLabels();
  const body = text(listing.detail.body);
  const tools = listing.mcp?.tools ?? [];
  const hasContents =
    tools.length > 0 ||
    Boolean(listing.plugin) ||
    Boolean(listing.connector) ||
    listing.kind === 'skill';
  return (
    <div className="flex flex-col gap-8">
      {/* The header already carries the short description; an empty body adds nothing to it. */}
      {body || !hasContents ? (
        <Section id="zgi-market-about" title={t('market.catalog.about')}>
          {body ? (
            // The body is written by whoever published the listing: raw HTML is never rendered.
            <MarkdownViewer content={body} allowRawHtml={false} />
          ) : (
            <p className="rounded-lg border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
              {t('market.catalog.detail.overview.empty')}
            </p>
          )}
        </Section>
      ) : null}
      {tools.length > 0 ? (
        <Section
          id="zgi-market-tools"
          title={t('market.catalog.detail.overview.toolsTitle')}
          aside={t('market.catalog.detail.overview.toolCount', { count: tools.length })}
        >
          <ZgiMarketToolList tools={tools} />
        </Section>
      ) : null}
      {listing.kind === 'skill' ? (
        <Section id="zgi-market-skill-contents" title={t('market.catalog.detail.skill.title')}>
          <ZgiMarketSkillContents
            slug={listing.slug}
            summary={listing.skill ?? null}
            shownDescription={text(listing.card.description)}
            activated={activatedIds.has(listing.id)}
          />
        </Section>
      ) : null}
      <PluginSummary listing={listing} />
      <ConnectorSummary listing={listing} />
      <Related listing={listing} activatedIds={activatedIds} />
    </div>
  );
}
