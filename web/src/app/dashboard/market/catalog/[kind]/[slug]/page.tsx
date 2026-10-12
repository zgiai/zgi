'use client';

import { use, useMemo, useState } from 'react';
import Link from 'next/link';
import { ArrowLeft, Check } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import MarketEmptyState from '@/components/market/market-empty-state';
import { MarketKindIcon } from '@/components/zgi-market/market-kind-icon';
import { marketGatewayOrigin } from '@/components/zgi-market/market-usage';
import { useMarketLabels } from '@/components/zgi-market/use-market-labels';
import { ZgiMarketApiSection } from '@/components/zgi-market/zgi-market-api-section';
import { ZgiMarketListingPanel } from '@/components/zgi-market/zgi-market-listing-panel';
import { ZgiMarketOverview } from '@/components/zgi-market/zgi-market-overview';
import { ZgiMarketPricingTable } from '@/components/zgi-market/zgi-market-pricing-table';
import { ZgiMarketRequiredConnectors } from '@/components/zgi-market/zgi-market-required-connectors';
import { ZgiMarketUsageGuide } from '@/components/zgi-market/zgi-market-usage-guide';
import {
  useActivateZgiMarketListing,
  useInstallZgiMarketSkill,
  useZgiMarketInstalledSkills,
  useZgiMarketListing,
  useZgiMarketMeta,
  useZgiMarketSubscriptions,
} from '@/hooks/use-zgi-market';
import {
  marketListingKinds,
  type MarketListingKind,
  type MarketOffering,
  type MarketRequiredConnector,
} from '@/services/types/zgi-market';

interface PageProps {
  params: Promise<{ kind: string; slug: string }>;
}

type DetailTab = 'overview' | 'api' | 'pricing' | 'usage';

/** Whether a failed read says the listing does not exist, rather than that the market failed. */
function isNotFound(error: unknown): boolean {
  if (!error || typeof error !== 'object') {
    return false;
  }
  const candidate = error as { status?: unknown; code?: unknown; response?: { status?: unknown } };
  return (
    candidate.status === 404 ||
    candidate.response?.status === 404 ||
    candidate.code === 404 ||
    candidate.code === '404'
  );
}

export default function ZgiMarketListingPage({ params }: PageProps) {
  const { kind: rawKind, slug } = use(params);
  const kind = (marketListingKinds as readonly string[]).includes(rawKind)
    ? (rawKind as MarketListingKind)
    : undefined;
  const { t, text } = useMarketLabels();
  const listing = useZgiMarketListing(kind, slug);
  const meta = useZgiMarketMeta();
  const subscriptions = useZgiMarketSubscriptions();
  const activate = useActivateZgiMarketListing();
  const installedSkills = useZgiMarketInstalledSkills(kind === 'skill');
  const install = useInstallZgiMarketSkill();
  // The organization's own skill a package would replace; the administrator decides.
  const [conflict, setConflict] = useState<string | null>(null);
  const [tab, setTab] = useState<DetailTab>('overview');

  const activeSubscriptions = useMemo(
    () => (subscriptions.data ?? []).filter(subscription => subscription.status === 'active'),
    [subscriptions.data]
  );
  const activatedIds = useMemo(
    () => new Set(activeSubscriptions.map(subscription => subscription.listing_id)),
    [activeSubscriptions]
  );

  const back = (
    <Link
      href="/dashboard/market/catalog"
      className="inline-flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
    >
      <ArrowLeft className="size-4" aria-hidden="true" />
      {t('market.catalog.back')}
    </Link>
  );

  if (!kind || listing.isError) {
    const missing = !kind || isNotFound(listing.error);
    return (
      <div className="flex h-full flex-col overflow-y-auto px-6 py-5">
        {back}
        <MarketEmptyState
          title={
            missing
              ? t('market.catalog.detail.errors.notFoundTitle')
              : t('market.catalog.detail.errors.loadFailedTitle')
          }
          actionLabel={missing ? undefined : t('market.catalog.retry')}
          onAction={missing ? undefined : () => void listing.refetch()}
        />
        <p className="-mt-10 pb-10 text-center text-sm text-muted-foreground" role="status">
          {!kind
            ? t('market.catalog.detail.errors.unknownKind')
            : missing
              ? t('market.catalog.detail.errors.notFoundDescription')
              : t('market.catalog.detail.errors.loadFailedDescription')}
        </p>
      </div>
    );
  }
  if (listing.isPending) {
    return (
      <div className="flex h-full flex-col gap-6 overflow-y-auto px-6 py-5">
        {back}
        <div className="flex gap-4">
          <Skeleton className="size-14 rounded-xl" />
          <div className="flex flex-1 flex-col gap-2">
            <Skeleton className="h-8 w-72 max-w-full" />
            <Skeleton className="h-4 w-full max-w-xl" />
          </div>
        </div>
        <div className="grid gap-8 lg:grid-cols-[minmax(0,1fr)_20rem]">
          <Skeleton className="h-96 w-full" />
          <Skeleton className="h-72 w-full" />
        </div>
      </div>
    );
  }

  const data = listing.data;
  const title = text(data.card.title) || data.slug;
  const subtitle = text(data.card.subtitle);
  const description = text(data.card.description);
  // A member who does not administer the organization reads everything and changes nothing. Until
  // the role is known the actions stay available: the market itself refuses a member's write.
  const canAct = meta.data?.admin !== false;
  const gateway = marketGatewayOrigin(meta.data?.gateway_url);

  const isActivated = (offering?: MarketOffering) =>
    activeSubscriptions.some(
      subscription =>
        subscription.listing_id === data.id &&
        (!offering || subscription.offering_id === offering.id)
    );
  const run = (offering?: MarketOffering) =>
    activate.mutate(
      { listingId: data.id, ...(offering ? { offeringId: offering.id } : {}) },
      {
        onSuccess: () => toast.success(t('market.catalog.activateDone')),
        onError: () => toast.error(t('market.catalog.activateFailed')),
      }
    );
  const installedSkill = (installedSkills.data ?? []).find(skill => skill.slug === data.slug);
  const installSkill = (overwrite = false) =>
    install.mutate(
      { slug: data.slug, overwrite },
      {
        onSuccess: result => {
          if (result.installed) {
            setConflict(null);
            toast.success(t('market.catalog.installDone'));
          } else if (result.conflict) {
            setConflict(result.conflict);
          } else {
            toast.error(
              t('market.catalog.installRefused', { reason: result.validation_errors?.[0] ?? '' })
            );
          }
        },
        onError: () => toast.error(t('market.catalog.installFailed')),
      }
    );

  const done = (label: string) => (
    <p className="flex h-10 items-center justify-center gap-1.5 rounded-md bg-success/10 text-sm font-medium text-success">
      <Check className="size-4" aria-hidden="true" />
      {label}
    </p>
  );
  const hint = (value: string) => (
    <p className="text-xs leading-5 text-muted-foreground">{value}</p>
  );
  const adminOnly = hint(t('market.catalog.detail.panel.adminOnly'));

  // A connector is connected, an API is activated per offering, a skill is installed: the market
  // activates it and its package becomes a skill of the organization. Everything else is activated
  // as a whole.
  let action: React.ReactNode;
  let actionNote: React.ReactNode = null;
  const selfConnector: MarketRequiredConnector[] =
    data.kind === 'connector'
      ? [{ slug: data.slug, auth_type: data.connector?.auth_type ?? '', title: data.card.title }]
      : [];
  if (data.kind === 'connector') {
    action = (
      <ZgiMarketRequiredConnectors
        kind={data.kind}
        slug={data.slug}
        connectors={selfConnector}
        canConnect={canAct}
        embedded
      />
    );
    actionNote = canAct ? hint(t('market.catalog.detail.panel.connectorHint')) : adminOnly;
  } else if (data.kind === 'skill') {
    action = installedSkill ? (
      done(
        installedSkill.version
          ? t('market.catalog.installedVersion', { version: installedSkill.version })
          : t('market.catalog.installed')
      )
    ) : canAct ? (
      <Button
        className="w-full"
        disabled={install.isPending || installedSkills.isPending}
        onClick={() => installSkill()}
      >
        {t('market.catalog.install')}
      </Button>
    ) : null;
    actionNote = (
      <>
        {installedSkill ? (
          <Link href="/console/skills" className="text-sm text-primary hover:underline">
            {t('market.catalog.manageSkills')}
          </Link>
        ) : canAct ? (
          hint(t('market.catalog.installHint'))
        ) : (
          adminOnly
        )}
        {conflict ? (
          <div role="alert" className="rounded-md border border-destructive/40 p-3">
            <p className="text-xs leading-5 text-destructive">
              {t('market.catalog.installConflict', { name: conflict })}
            </p>
            <Button
              size="sm"
              variant="outline"
              className="mt-2"
              disabled={install.isPending}
              onClick={() => installSkill(true)}
            >
              {t('market.catalog.installOverwrite')}
            </Button>
          </div>
        ) : null}
      </>
    );
  } else if (data.kind === 'api') {
    const activatedCount = data.offerings.filter(offering => isActivated(offering)).length;
    const only = data.offerings.length === 1 ? data.offerings[0] : undefined;
    if (only) {
      action = isActivated(only) ? (
        done(t('market.catalog.activated'))
      ) : canAct ? (
        <Button className="w-full" disabled={activate.isPending} onClick={() => run(only)}>
          {t('market.catalog.activate')}
        </Button>
      ) : null;
      actionNote = canAct ? null : adminOnly;
    } else {
      action = (
        <>
          {activatedCount > 0
            ? done(
                t('market.catalog.detail.panel.activatedOfferings', {
                  count: activatedCount,
                  total: data.offerings.length,
                })
              )
            : null}
          {activatedCount < data.offerings.length ? (
            <Button
              className="w-full"
              variant={activatedCount > 0 ? 'outline' : 'default'}
              onClick={() => setTab('api')}
            >
              {t('market.catalog.detail.panel.chooseOffering')}
            </Button>
          ) : null}
        </>
      );
      actionNote = canAct ? hint(t('market.catalog.detail.panel.perOffering')) : adminOnly;
    }
  } else {
    action = isActivated() ? (
      done(t('market.catalog.activated'))
    ) : canAct ? (
      <Button className="w-full" disabled={activate.isPending} onClick={() => run()}>
        {t('market.catalog.activate')}
      </Button>
    ) : null;
    actionNote = !canAct
      ? adminOnly
      : data.kind === 'mcp'
        ? hint(t('market.catalog.detail.panel.mcpHint'))
        : data.kind === 'plugin'
          ? hint(t('market.catalog.detail.panel.pluginHint'))
          : null;
  }

  const requiredConnectors = data.kind === 'connector' ? [] : (data.required_connectors ?? []);
  const provider = [...new Set(data.offerings.map(offering => offering.provider).filter(Boolean))];
  const author = data.plugin?.manifest?.author;
  const byline = provider.length > 0 ? provider.join(' · ') : author;

  return (
    <div className="flex h-full flex-col overflow-y-auto px-4 py-5 sm:px-6">
      {back}
      <header className="mt-4 flex gap-4">
        <MarketKindIcon kind={data.kind} size="lg" />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
            <h1 className="min-w-0 break-words text-2xl font-semibold leading-tight">{title}</h1>
            <div className="flex flex-wrap items-center gap-1.5">
              <Badge variant="secondary">{t(`market.catalog.kinds.${data.kind}`)}</Badge>
              {data.current_version?.label ? (
                <Badge variant="outline">{data.current_version.label}</Badge>
              ) : null}
              {data.card.badges.map(badge => (
                <Badge key={badge} variant="info">
                  {badge}
                </Badge>
              ))}
            </div>
          </div>
          {subtitle ? <p className="mt-1.5 text-sm font-medium">{subtitle}</p> : null}
          {description && description !== subtitle ? (
            <p className="mt-1.5 max-w-3xl text-sm leading-6 text-muted-foreground">
              {description}
            </p>
          ) : null}
          <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1.5 text-xs text-muted-foreground">
            {byline ? (
              <span>{t('market.catalog.detail.providedBy', { provider: byline })}</span>
            ) : null}
            {data.primary_category ? <span>{text(data.primary_category.name)}</span> : null}
            {data.tags.map(tag => (
              <Badge key={tag} variant="subtle">
                {tag}
              </Badge>
            ))}
          </div>
        </div>
      </header>

      <div className="mt-6 grid items-start gap-6 pb-8 lg:grid-cols-[minmax(0,1fr)_20rem] lg:gap-8">
        <div className="order-last min-w-0 lg:order-none">
          <Tabs value={tab} onValueChange={value => setTab(value as DetailTab)}>
            <TabsList className="h-auto max-w-full flex-wrap justify-start">
              <TabsTrigger value="overview">{t('market.catalog.detail.tabs.overview')}</TabsTrigger>
              {data.kind === 'api' ? (
                <TabsTrigger value="api">
                  {t('market.catalog.detail.tabs.api')}
                  <span className="ml-1.5 text-xs text-muted-foreground">
                    {data.offerings.length}
                  </span>
                </TabsTrigger>
              ) : null}
              {data.kind === 'connector' ? null : (
                <TabsTrigger value="pricing">{t('market.catalog.detail.tabs.pricing')}</TabsTrigger>
              )}
              <TabsTrigger value="usage">{t('market.catalog.detail.tabs.usage')}</TabsTrigger>
            </TabsList>
            <TabsContent value="overview" className="mt-5">
              <ZgiMarketOverview listing={data} activatedIds={activatedIds} />
            </TabsContent>
            {data.kind === 'api' ? (
              <TabsContent value="api" className="mt-5">
                <ZgiMarketApiSection
                  listing={data}
                  gatewayOrigin={gateway.origin}
                  isActivated={isActivated}
                  onActivate={canAct ? run : undefined}
                  activating={activate.isPending}
                />
              </TabsContent>
            ) : null}
            {data.kind === 'connector' ? null : (
              <TabsContent value="pricing" className="mt-5">
                <ZgiMarketPricingTable listing={data} />
              </TabsContent>
            )}
            <TabsContent value="usage" className="mt-5">
              <ZgiMarketUsageGuide listing={data} gateway={gateway} />
            </TabsContent>
          </Tabs>
        </div>

        <ZgiMarketListingPanel listing={data} action={action}>
          {actionNote}
          {requiredConnectors.length > 0 ? (
            <div className="flex flex-col gap-2 border-t pt-4">
              <h2 className="text-sm font-semibold">{t('market.catalog.connectors.title')}</h2>
              <p className="text-xs leading-5 text-muted-foreground">
                {t('market.catalog.connectors.description')}
              </p>
              <ZgiMarketRequiredConnectors
                kind={data.kind}
                slug={data.slug}
                connectors={requiredConnectors}
                canConnect={canAct}
                embedded
              />
            </div>
          ) : null}
        </ZgiMarketListingPanel>
      </div>
    </div>
  );
}
