'use client';

import type { ReactNode } from 'react';
import Link from 'next/link';
import { ArrowUpRight } from 'lucide-react';
import type { MarketListing } from '@/services/types/zgi-market';
import {
  MARKET_CREDENTIAL_ENV,
  marketApiQuickStart,
  marketMcpClientConfig,
  marketMcpServerUrl,
  marketMcpToolsListCurl,
} from './market-usage';
import { useMarketLabels } from './use-market-labels';
import { ZgiMarketCodeSample } from './zgi-market-code-sample';

function Step({ index, title, children }: { index: number; title: string; children: ReactNode }) {
  return (
    <li className="grid grid-cols-[1.75rem_minmax(0,1fr)] gap-x-3">
      <span
        className="flex size-7 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold text-primary"
        aria-hidden="true"
      >
        {index}
      </span>
      <div className="flex min-w-0 flex-col gap-3 pb-1">
        <h4 className="pt-1 text-sm font-semibold">{title}</h4>
        {children}
      </div>
    </li>
  );
}

function Prose({ children }: { children: ReactNode }) {
  return <p className="text-sm leading-6 text-muted-foreground">{children}</p>;
}

function PageLink({ href, children }: { href: string; children: ReactNode }) {
  return (
    <Link
      href={href}
      className="inline-flex items-center gap-1 text-sm font-medium text-primary hover:underline"
    >
      {children}
      <ArrowUpRight className="size-3.5" aria-hidden="true" />
    </Link>
  );
}

function Block({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-4 rounded-lg border bg-background p-4">
      <h3 className="text-sm font-semibold">{title}</h3>
      {children}
    </section>
  );
}

interface ZgiMarketUsageGuideProps {
  listing: MarketListing;
  gateway: { origin: string; configured: boolean };
}

/** How the organization uses the listing once it has it: inside this product, and from outside. */
export function ZgiMarketUsageGuide({ listing, gateway }: ZgiMarketUsageGuideProps) {
  const { t } = useMarketLabels();
  const needsConnection = (listing.required_connectors ?? []).length > 0;
  const notes = (
    <div className="flex flex-col gap-2 text-xs leading-5 text-muted-foreground">
      <p>{t('market.catalog.detail.usage.credential', { env: MARKET_CREDENTIAL_ENV })}</p>
      {gateway.configured ? null : (
        <p className="rounded-md border border-dashed px-3 py-2">
          {t('market.catalog.detail.usage.placeholder', { origin: gateway.origin })}
        </p>
      )}
    </div>
  );

  if (listing.kind === 'api') {
    const offering = listing.offerings[0];
    if (!offering) {
      return null;
    }
    const quickStart = marketApiQuickStart(gateway.origin, offering);
    return (
      <ol className="flex flex-col gap-6">
        <Step index={1} title={t('market.catalog.detail.usage.stepActivate')}>
          <Prose>{t('market.catalog.detail.usage.stepActivateApi')}</Prose>
        </Step>
        <Step index={2} title={t('market.catalog.detail.usage.stepCall')}>
          <Prose>{t('market.catalog.detail.usage.stepCallBody')}</Prose>
          <ZgiMarketCodeSample
            label={t('market.catalog.detail.usage.requestExample')}
            code={quickStart.curl}
          />
          {quickStart.needsModel ? (
            <Prose>{t('market.catalog.detail.usage.modelNote', { placeholder: '<model>' })}</Prose>
          ) : null}
          {quickStart.concrete ? null : (
            <Prose>{t('market.catalog.detail.usage.genericNote')}</Prose>
          )}
          {listing.offerings.length > 1 ? (
            <Prose>
              {t('market.catalog.detail.usage.otherOfferings', { offering: offering.slug })}
            </Prose>
          ) : null}
          {notes}
        </Step>
      </ol>
    );
  }

  if (listing.kind === 'mcp') {
    return (
      <div className="flex flex-col gap-4">
        <Block title={t('market.catalog.detail.usage.inZgi')}>
          <ol className="flex flex-col gap-6">
            <Step index={1} title={t('market.catalog.detail.usage.stepActivate')}>
              <Prose>{t('market.catalog.detail.usage.stepActivateMcp')}</Prose>
            </Step>
            {needsConnection ? (
              <Step index={2} title={t('market.catalog.detail.usage.stepConnect')}>
                <Prose>{t('market.catalog.detail.usage.stepConnectBody')}</Prose>
              </Step>
            ) : null}
            <Step index={needsConnection ? 3 : 2} title={t('market.tools.title')}>
              <Prose>{t('market.catalog.detail.usage.inZgiMcp')}</Prose>
              <div className="flex flex-wrap gap-x-5 gap-y-2">
                <PageLink href="/console/agents">
                  {t('market.catalog.detail.usage.openAgents')}
                </PageLink>
                <PageLink href="/console/digital-humans">
                  {t('market.catalog.detail.usage.openDigitalHumans')}
                </PageLink>
              </div>
            </Step>
          </ol>
        </Block>
        <Block title={t('market.catalog.detail.usage.external')}>
          <Prose>{t('market.catalog.detail.usage.externalBody')}</Prose>
          <dl className="grid gap-x-4 gap-y-1 text-sm sm:grid-cols-[auto_minmax(0,1fr)]">
            <dt className="text-muted-foreground">{t('market.catalog.detail.usage.serverUrl')}</dt>
            <dd className="break-all font-mono text-xs leading-6">
              {marketMcpServerUrl(gateway.origin, listing.slug)}
            </dd>
          </dl>
          <ZgiMarketCodeSample
            label={t('market.catalog.detail.usage.clientConfig')}
            code={marketMcpClientConfig(gateway.origin, listing.slug)}
          />
          <ZgiMarketCodeSample
            label={t('market.catalog.detail.usage.toolsExample')}
            code={marketMcpToolsListCurl(gateway.origin, listing.slug)}
          />
          {notes}
        </Block>
      </div>
    );
  }

  if (listing.kind === 'skill') {
    return (
      <ol className="flex flex-col gap-6">
        <Step index={1} title={t('market.catalog.detail.usage.skillInstall')}>
          <Prose>{t('market.catalog.detail.usage.skillInstallBody')}</Prose>
          <PageLink href="/console/skills">{t('market.catalog.detail.usage.openSkills')}</PageLink>
        </Step>
        <Step index={2} title={t('market.catalog.detail.usage.skillBind')}>
          <Prose>{t('market.catalog.detail.usage.skillBindBody')}</Prose>
          <div className="flex flex-wrap gap-x-5 gap-y-2">
            <PageLink href="/console/agents">
              {t('market.catalog.detail.usage.openAgents')}
            </PageLink>
            <PageLink href="/console/digital-humans">
              {t('market.catalog.detail.usage.openDigitalHumans')}
            </PageLink>
          </div>
        </Step>
      </ol>
    );
  }

  return (
    <Prose>
      {listing.kind === 'plugin'
        ? t('market.catalog.detail.usage.pluginBody')
        : listing.kind === 'connector'
          ? t('market.catalog.detail.usage.connectorBody')
          : t('market.catalog.detail.usage.datasetBody')}
    </Prose>
  );
}
