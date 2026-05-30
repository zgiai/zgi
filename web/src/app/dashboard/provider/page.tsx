'use client';

import React from 'react';
import Link from 'next/link';
import { useT } from '@/i18n';
import {
  useProviders,
  useCustomProviders,
  useCreateCustomProvider,
  useUpdateCustomProvider,
  useDeleteCustomProvider,
} from '@/hooks/provider/use-provider';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Badge } from '@/components/ui/badge';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { ProviderIcon } from '@/components/common/provider-icon';
import {
  Plus,
  Edit,
  Trash2,
  ArrowRight,
  Cable,
  Boxes,
  RadioTower,
  Brain,
  Factory,
  CheckCircle2,
  AlertCircle,
  X,
} from 'lucide-react';
import { CustomProviderDialog } from '@/components/providers/custom-provider-dialog';
import type {
  CreateCustomProviderRequest,
  ProviderItem,
  UpdateCustomProviderRequest,
} from '@/services/types/provider';
import { useProviderDisplay } from '@/hooks/provider/use-provider-display';
import { IS_CLOUD } from '@/lib/config';
import { ProviderSyncButton } from '@/components/providers/provider-sync-button';
import { useProviderI18n } from '@/hooks/provider/use-provider-i18n';
import { useProviderAvailableCounts } from '@/hooks/provider/use-provider-available-counts';
import { getProviderRuntimeState } from '@/utils/provider-runtime-state';

function SummaryCard({
  title,
  value,
  description,
  icon: Icon,
}: {
  title: string;
  value: string;
  description: string;
  icon: React.ElementType;
}) {
  return (
    <Card className="border-border/70 shadow-sm">
      <CardContent className="flex items-start justify-between gap-4 p-5">
        <div className="space-y-1">
          <p className="text-sm font-medium text-muted-foreground">{title}</p>
          <p className="text-2xl font-semibold tracking-tight text-foreground">{value}</p>
          <p className="text-xs text-muted-foreground">{description}</p>
        </div>
        <div className="flex h-10 w-10 items-center justify-center rounded-2xl bg-primary/10 text-primary">
          <Icon className="h-4.5 w-4.5" />
        </div>
      </CardContent>
    </Card>
  );
}

function ChannelStatus({
  provider,
  availableModelCount,
}: {
  provider: ProviderItem;
  availableModelCount: number;
}) {
  const t = useT('aiProviders');
  const channelCount = provider.channel_count ?? 0;
  const hasUsableModels = availableModelCount > 0;

  return (
    <div className="space-y-1">
      <Badge
        variant="secondary"
        className={
          channelCount > 0
            ? 'h-7 rounded-sm bg-success/15 px-3 text-[12px] font-semibold text-success ring-1 ring-success/10'
            : 'h-7 rounded-sm bg-warning/15 px-3 text-[12px] font-semibold text-warning ring-1 ring-warning/10'
        }
      >
        {channelCount > 0
          ? t('providersList.channelStatus.configured', { count: channelCount })
          : t('providersList.channelStatus.notConfigured')}
      </Badge>
      <p className="text-xs text-muted-foreground">
        {channelCount > 0
          ? hasUsableModels
            ? t('providersList.channelStatus.readyHint')
            : t('providersList.channelStatus.noUsableModelsHint')
          : t('providersList.channelStatus.configureHint')}
      </p>
    </div>
  );
}

function ProviderStatusBadge({
  provider,
  availableModelCount,
}: {
  provider: ProviderItem;
  availableModelCount: number;
}) {
  const t = useT('aiProviders');
  const state = getProviderRuntimeState(provider, availableModelCount);
  const modelCount = provider.model_count ?? 0;

  const badgeContent =
    state === 'available_models'
      ? {
          icon: CheckCircle2,
          label: t('providersList.badges.availableCount', {
            count: availableModelCount,
          }),
          className:
            'border-transparent bg-success/15 text-success shadow-none ring-1 ring-success/10',
        }
      : state === 'pending_channels'
        ? {
            icon: AlertCircle,
            label: t('providersList.runtimeStates.pending_channels'),
            className:
              'border-transparent bg-warning/15 text-warning shadow-none ring-1 ring-warning/10',
          }
        : state === 'no_catalog_models'
          ? {
              icon: Boxes,
              label: t('providersList.runtimeStates.no_catalog_models'),
              className:
                'border-border bg-muted/70 text-muted-foreground shadow-none ring-1 ring-border/60',
            }
          : {
              icon: X,
              label: t('providersList.runtimeStates.disabled'),
              className:
                'border-border bg-muted/70 text-muted-foreground shadow-none ring-1 ring-border/60',
            };

  const Icon = badgeContent.icon;

  return (
    <div className="space-y-1">
      <Badge className={`h-7 px-3 text-[12px] font-semibold ${badgeContent.className}`}>
        <Icon className="h-3.5 w-3.5" />
        {badgeContent.label}
      </Badge>
      <p className="text-xs text-muted-foreground">
        {state === 'disabled'
          ? t('providersList.runtimeStateHints.disabled')
          : state === 'available_models'
            ? t('providersList.runtimeStateHints.available_models')
            : state === 'pending_channels'
              ? t('providersList.runtimeStateHints.pending_channels', { count: modelCount })
              : t('providersList.runtimeStateHints.no_catalog_models')}
      </p>
    </div>
  );
}

function OfficialProviderRow({
  provider,
  availableModelCount,
}: {
  provider: ProviderItem;
  availableModelCount: number;
}) {
  const t = useT('aiProviders');
  const { name, description } = useProviderDisplay(provider);

  return (
    <TableRow>
      <TableCell className="min-w-[18rem]">
        <div className="flex items-start gap-3">
          <ProviderIcon provider={provider.provider} size={32} />
          <div className="min-w-0 space-y-1">
            <div className="font-medium text-sm text-foreground">{name}</div>
            <p className="line-clamp-2 whitespace-normal text-xs leading-5 text-muted-foreground">
              {description || t('providersList.table.noDescription')}
            </p>
          </div>
        </div>
      </TableCell>
      <TableCell>
        <ProviderStatusBadge provider={provider} availableModelCount={availableModelCount} />
      </TableCell>
      <TableCell>
        <ChannelStatus provider={provider} availableModelCount={availableModelCount} />
      </TableCell>
      <TableCell>{provider.model_count ?? 0}</TableCell>
      <TableCell className="text-right">
        <div className="flex items-center justify-end gap-2">
          <Button asChild size="sm" variant="outline" className="gap-1.5">
            <Link href={`/dashboard/provider/${encodeURIComponent(provider.provider)}`}>
              {t('providersList.table.viewModels')}
              <ArrowRight className="h-3.5 w-3.5" />
            </Link>
          </Button>
          <Button asChild size="sm" variant="ghost" className="gap-1.5 text-primary">
            <Link href="/dashboard/channel">
              <RadioTower className="h-3.5 w-3.5" />
              {t('providersList.table.configureChannel')}
            </Link>
          </Button>
        </div>
      </TableCell>
    </TableRow>
  );
}

function CustomProviderRow({
  provider,
  availableModelCount,
  onEdit,
  onDelete,
}: {
  provider: ProviderItem;
  availableModelCount: number;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const t = useT('aiProviders');
  const getProviderName = useProviderI18n();

  return (
    <TableRow>
      <TableCell className="min-w-[16rem]">
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <span className="font-medium text-sm text-foreground">
              {getProviderName(provider.provider, provider.provider_name)}
            </span>
            <Badge variant="info">{t('providersList.table.custom')}</Badge>
          </div>
          <p className="line-clamp-1 whitespace-normal break-all text-xs text-muted-foreground">
            {provider.api_base_url || t('providersList.table.noEndpoint')}
          </p>
        </div>
      </TableCell>
      <TableCell>
        <ProviderStatusBadge provider={provider} availableModelCount={availableModelCount} />
      </TableCell>
      <TableCell>
        <ChannelStatus provider={provider} availableModelCount={availableModelCount} />
      </TableCell>
      <TableCell className="min-w-[18rem]">
        <p className="line-clamp-2 whitespace-normal text-xs leading-5 text-muted-foreground">
          {provider.description || t('providersList.table.noDescription')}
        </p>
      </TableCell>
      <TableCell>{provider.model_count ?? 0}</TableCell>
      <TableCell className="text-right">
        <div className="flex items-center justify-end gap-2">
          <Button asChild size="sm" variant="outline" className="gap-1.5">
            <Link href={`/dashboard/provider/${encodeURIComponent(provider.provider)}`}>
              {t('providersList.table.viewModels')}
              <ArrowRight className="h-3.5 w-3.5" />
            </Link>
          </Button>
          <Button asChild size="sm" variant="ghost" className="gap-1.5 text-primary">
            <Link href="/dashboard/channel">
              <RadioTower className="h-3.5 w-3.5" />
              {t('providersList.table.configureChannel')}
            </Link>
          </Button>
          <Button
            variant="ghost"
            isIcon
            className="h-8 w-8 text-muted-foreground hover:text-primary"
            onClick={onEdit}
          >
            <Edit className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            isIcon
            className="h-8 w-8 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
            onClick={onDelete}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </TableCell>
    </TableRow>
  );
}

export default function ProviderPage() {
  const t = useT('aiProviders');
  const getProviderName = useProviderI18n();
  const { items: officialItems, isLoading: isOfficialLoading } = useProviders();
  const { items: customItems, isLoading: isCustomLoading } = useCustomProviders();
  const { createCustomProvider, isCreating } = useCreateCustomProvider();
  const { updateCustomProvider, isUpdating } = useUpdateCustomProvider();
  const { deleteCustomProvider, isDeleting } = useDeleteCustomProvider();

  const [isDialogOpen, setIsDialogOpen] = React.useState(false);
  const [editingProvider, setEditingProvider] = React.useState<ProviderItem | undefined>(undefined);
  const [deletingProvider, setDeletingProvider] = React.useState<ProviderItem | undefined>(
    undefined
  );

  const isLoading = isOfficialLoading || isCustomLoading;

  const customProviders = React.useMemo(
    () => [...customItems].sort((a, b) => Number(b.is_enabled) - Number(a.is_enabled)),
    [customItems]
  );
  const officialProviders = React.useMemo(
    () => [...officialItems].sort((a, b) => Number(b.is_enabled) - Number(a.is_enabled)),
    [officialItems]
  );
  const allProviders = React.useMemo(
    () => [...officialProviders, ...customProviders],
    [customProviders, officialProviders]
  );
  const { counts: availableCounts, isLoading: isLoadingAvailableCounts } =
    useProviderAvailableCounts(allProviders);

  const summary = React.useMemo(() => {
    const counters = {
      availableProviders: 0,
      pendingChannelProviders: 0,
      noCatalogProviders: 0,
      disabledProviders: 0,
      totalChannels: 0,
      availableModels: 0,
    };

    allProviders.forEach(provider => {
      const availableModelCount = availableCounts[provider.provider] ?? 0;
      const state = getProviderRuntimeState(provider, availableModelCount);
      if (state === 'available_models') counters.availableProviders += 1;
      if (state === 'pending_channels') counters.pendingChannelProviders += 1;
      if (state === 'no_catalog_models') counters.noCatalogProviders += 1;
      if (state === 'disabled') counters.disabledProviders += 1;
      counters.totalChannels += provider.channel_count ?? 0;
      counters.availableModels += availableModelCount;
    });

    return {
      ...counters,
      totalProviders: allProviders.length,
      customProviders: customProviders.length,
      totalModels: allProviders.reduce((sum, provider) => sum + (provider.model_count ?? 0), 0),
    };
  }, [allProviders, availableCounts, customProviders.length]);

  const handleCreateOrUpdate = async (
    data: CreateCustomProviderRequest | UpdateCustomProviderRequest
  ) => {
    if (editingProvider) {
      await updateCustomProvider(editingProvider.id, data as UpdateCustomProviderRequest);
    } else {
      await createCustomProvider(data as CreateCustomProviderRequest);
    }
  };

  const handleDelete = async () => {
    if (deletingProvider) {
      await deleteCustomProvider(deletingProvider.id);
      setDeletingProvider(undefined);
    }
  };

  if (isLoading || isLoadingAvailableCounts) {
    return (
      <div className="mx-auto max-w-6xl space-y-6">
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          {Array.from({ length: 4 }).map((_, index) => (
            <Card key={index}>
              <CardContent className="p-5">
                <Skeleton className="h-4 w-24" />
                <Skeleton className="mt-3 h-8 w-20" />
                <Skeleton className="mt-2 h-4 w-36" />
              </CardContent>
            </Card>
          ))}
        </div>
        <Card>
          <CardHeader>
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-4 w-80" />
          </CardHeader>
          <CardContent>
            <Skeleton className="h-72 w-full" />
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <section className="rounded-md border bg-background p-6 shadow-sm">
        <div className="flex flex-col gap-5 lg:flex-row lg:items-start lg:justify-between">
          <div className="max-w-3xl space-y-3">
            <div className="space-y-1">
              <p className="text-sm font-medium text-muted-foreground">
                {t('providersList.eyebrow')}
              </p>
              <h1 className="text-2xl font-semibold tracking-tight">{t('providersList.title')}</h1>
              <p className="text-sm leading-6 text-muted-foreground">
                {t('providersList.description')}
              </p>
            </div>
            <div className="grid gap-3 md:grid-cols-2">
              <div className="rounded-md border bg-bg-canvas/60 p-3">
                <div className="text-sm font-medium text-foreground">
                  {t('providersList.concepts.providerTitle')}
                </div>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                  {t('providersList.concepts.providerDescription')}
                </p>
              </div>
              <div className="rounded-md border bg-bg-canvas/60 p-3">
                <div className="text-sm font-medium text-foreground">
                  {t('providersList.concepts.channelTitle')}
                </div>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                  {t('providersList.concepts.channelDescription')}
                </p>
              </div>
            </div>
          </div>
          <div className="flex shrink-0 flex-col gap-2 sm:flex-row lg:flex-col">
            <Button asChild className="gap-2">
              <Link href="/dashboard/channel">
                <RadioTower className="h-4 w-4" />
                {t('providersList.actions.configureChannel')}
              </Link>
            </Button>
            <Button
              variant="outline"
              className="gap-2"
              onClick={() => {
                setEditingProvider(undefined);
                setIsDialogOpen(true);
              }}
            >
              <Plus className="h-4 w-4" />
              {t('providersList.table.addCustomProvider')}
            </Button>
          </div>
        </div>
      </section>

      <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <SummaryCard
          title={t('providersList.summary.totalTitle')}
          value={String(summary.totalProviders)}
          description={t('providersList.summary.totalDescription')}
          icon={Factory}
        />
        <SummaryCard
          title={t('providersList.summary.channelsTitle')}
          value={String(summary.totalChannels)}
          description={t('providersList.summary.channelsDescription')}
          icon={RadioTower}
        />
        <SummaryCard
          title={t('providersList.summary.availableModelsTitle')}
          value={String(summary.availableModels)}
          description={t('providersList.summary.availableModelsDescription')}
          icon={Brain}
        />
        <SummaryCard
          title={t('providersList.summary.pendingTitle')}
          value={String(summary.pendingChannelProviders)}
          description={t('providersList.summary.pendingDescription')}
          icon={Cable}
        />
      </section>

      <Card className="border-border/70 shadow-sm">
        <CardHeader className="flex flex-row items-start justify-between gap-4">
          <div className="space-y-1.5">
            <CardTitle>{t('providersList.customTitle')}</CardTitle>
            <CardDescription>{t('providersList.customDescription')}</CardDescription>
          </div>
          <Button
            size="sm"
            onClick={() => {
              setEditingProvider(undefined);
              setIsDialogOpen(true);
            }}
          >
            <Plus className="mr-2 h-4 w-4" />
            {t('providersList.table.addCustomProvider')}
          </Button>
        </CardHeader>
        <CardContent>
          {customProviders.length === 0 ? (
            <div className="rounded-xl border border-dashed border-border/80 bg-muted/20 px-6 py-10 text-center">
              <p className="text-sm font-medium text-foreground">
                {t('providersList.empty.customTitle')}
              </p>
              <p className="mt-2 text-sm text-muted-foreground">
                {t('providersList.empty.customDescription')}
              </p>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('providersList.table.provider')}</TableHead>
                  <TableHead>{t('providersList.table.status')}</TableHead>
                  <TableHead>{t('providersList.table.channels')}</TableHead>
                  <TableHead>{t('providersList.table.description')}</TableHead>
                  <TableHead>{t('providersList.table.models')}</TableHead>
                  <TableHead className="text-right">{t('providersList.table.actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {customProviders.map(provider => (
                  <CustomProviderRow
                    key={provider.id}
                    provider={provider}
                    availableModelCount={availableCounts[provider.provider] ?? 0}
                    onEdit={() => {
                      setEditingProvider(provider);
                      setIsDialogOpen(true);
                    }}
                    onDelete={() => setDeletingProvider(provider)}
                  />
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Card className="border-border/70 shadow-sm">
        <CardHeader className="flex flex-row items-start justify-between gap-4">
          <div className="space-y-1.5">
            <CardTitle>{t('providersList.officialTitle')}</CardTitle>
            <CardDescription>{t('providersList.officialDescription')}</CardDescription>
          </div>
          {!IS_CLOUD && <ProviderSyncButton />}
        </CardHeader>
        <CardContent>
          {officialProviders.length === 0 ? (
            <div className="rounded-xl border border-dashed border-border/80 bg-muted/20 px-6 py-10 text-center">
              <p className="text-sm font-medium text-foreground">
                {t('providersList.empty.title')}
              </p>
              <p className="mt-2 text-sm text-muted-foreground">
                {t('providersList.empty.description')}
              </p>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('providersList.table.provider')}</TableHead>
                  <TableHead>{t('providersList.table.status')}</TableHead>
                  <TableHead>{t('providersList.table.channels')}</TableHead>
                  <TableHead>{t('providersList.table.models')}</TableHead>
                  <TableHead className="text-right">{t('providersList.table.actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {officialProviders.map(provider => (
                  <OfficialProviderRow
                    key={provider.id}
                    provider={provider}
                    availableModelCount={availableCounts[provider.provider] ?? 0}
                  />
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <CustomProviderDialog
        open={isDialogOpen}
        onOpenChange={setIsDialogOpen}
        initialData={editingProvider}
        onSubmit={handleCreateOrUpdate}
        isSubmitting={isCreating || isUpdating}
      />

      <ConfirmDialog
        variant="danger"
        open={!!deletingProvider}
        onOpenChange={(open: boolean) => !open && setDeletingProvider(undefined)}
        title={t('custom.delete.title')}
        description={t('custom.delete.content', {
          name: getProviderName(deletingProvider?.provider, deletingProvider?.provider_name),
        })}
        confirmText={
          isDeleting ? (t('actions.saving') as string) : (t('custom.delete.confirm') as string)
        }
        cancelText={t('custom.delete.cancel') as string}
        onConfirm={handleDelete}
        loading={isDeleting}
      />
    </div>
  );
}
