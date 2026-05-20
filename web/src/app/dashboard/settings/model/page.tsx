'use client';

import { type ComponentType, useCallback, useEffect, useMemo, useState } from 'react';
import { useRouter } from 'next/navigation';
import { useT } from '@/i18n';
import ModelSelectorParameter from '@/components/common/model-selector/model-selector-parameter';
import type { ModelSelectorParameterValue } from '@/components/common/model-selector/model-selector-parameter';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import {
  AlertTriangle,
  ArrowUpDown,
  Database,
  Eye,
  Image as ImageIcon,
  Loader2,
  MessageSquare,
  Save,
} from 'lucide-react';
import {
  EMPTY_DEFAULT_MODEL_VALUE,
  createEmptyDefaultModelSettings,
  useDefaultModels,
  type DefaultModelSettings,
  type ResolvedDefaultModel,
} from '@/hooks/model/use-default-models';
import type { DefaultModelUseCase } from '@/services/types/model';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';

const _DISPLAY_DEFAULT_MODEL_USE_CASES = [
  'text-chat',
  'embedding',
  'rerank',
  'vision',
  'image-gen',
] as const satisfies readonly DefaultModelUseCase[];

type DisplayDefaultModelUseCase = (typeof _DISPLAY_DEFAULT_MODEL_USE_CASES)[number];

const USE_CASE_CARDS: Array<{
  key: DisplayDefaultModelUseCase;
  icon: ComponentType<{ className?: string }>;
  iconBg: string;
  iconColor: string;
}> = [
  {
    key: 'text-chat',
    icon: MessageSquare,
    iconBg: 'bg-primary/10',
    iconColor: 'text-primary',
  },
  {
    key: 'embedding',
    icon: Database,
    iconBg: 'bg-violet-500/10',
    iconColor: 'text-violet-500',
  },
  {
    key: 'rerank',
    icon: ArrowUpDown,
    iconBg: 'bg-amber-500/10',
    iconColor: 'text-amber-500',
  },
  {
    key: 'vision',
    icon: Eye,
    iconBg: 'bg-orange-500/10',
    iconColor: 'text-orange-500',
  },
  {
    key: 'image-gen',
    icon: ImageIcon,
    iconBg: 'bg-fuchsia-500/10',
    iconColor: 'text-fuchsia-500',
  },
];

function shallowEqualParams(
  a: Record<string, number | string | boolean>,
  b: Record<string, number | string | boolean>
): boolean {
  const aKeys = Object.keys(a);
  const bKeys = Object.keys(b);

  if (aKeys.length !== bKeys.length) {
    return false;
  }

  return aKeys.every(key => a[key] === b[key]);
}

function isSameValue(
  a: DefaultModelSettings[DisplayDefaultModelUseCase],
  b: DefaultModelSettings[DisplayDefaultModelUseCase]
): boolean {
  return (
    a.provider === b.provider &&
    a.model === b.model &&
    shallowEqualParams(a.params ?? {}, b.params ?? {})
  );
}

function getSourceBadgeVariant(source: ResolvedDefaultModel['source'] | 'pendingDelete') {
  if (source === 'explicit') {
    return 'success';
  }
  if (source === 'auto') {
    return 'info';
  }
  if (source === 'pendingDelete') {
    return 'warning';
  }
  return 'outline';
}

export default function ModelSettingsPage() {
  const t = useT();
  const router = useRouter();
  const {
    settings,
    resolvedSettings,
    isLoading,
    isError,
    error,
    refetch,
    updateDefaultModels,
    isUpdating,
  } = useDefaultModels();
  const [config, setConfig] = useState<DefaultModelSettings>(() => createEmptyDefaultModelSettings());
  const [leaveConfirmOpen, setLeaveConfirmOpen] = useState(false);
  const [pendingHref, setPendingHref] = useState<string | null>(null);

  useEffect(() => {
    if (!isLoading) {
      setConfig(settings);
    }
  }, [isLoading, settings]);

  const isDirty = useMemo(
    () => USE_CASE_CARDS.some(({ key }) => !isSameValue(config[key], settings[key])),
    [config, settings]
  );

  useEffect(() => {
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!isDirty || isUpdating) return;
      event.preventDefault();
      event.returnValue = '';
    };

    window.addEventListener('beforeunload', handleBeforeUnload);
    return () => window.removeEventListener('beforeunload', handleBeforeUnload);
  }, [isDirty, isUpdating]);

  useEffect(() => {
    if (!isDirty || isUpdating) return;

    const handleDocumentClick = (event: MouseEvent) => {
      if (
        event.defaultPrevented ||
        event.button !== 0 ||
        event.metaKey ||
        event.ctrlKey ||
        event.shiftKey ||
        event.altKey
      ) {
        return;
      }

      const target = event.target;
      if (!(target instanceof HTMLElement)) return;

      const anchor = target.closest('a[href]');
      if (!(anchor instanceof HTMLAnchorElement)) return;
      if (anchor.target === '_blank' || anchor.hasAttribute('download')) return;

      const href = anchor.getAttribute('href');
      if (!href || href.startsWith('#')) return;

      const nextUrl = new URL(anchor.href, window.location.href);
      const currentUrl = new URL(window.location.href);

      if (nextUrl.origin !== currentUrl.origin || nextUrl.href === currentUrl.href) {
        return;
      }

      event.preventDefault();
      event.stopPropagation();
      setPendingHref(`${nextUrl.pathname}${nextUrl.search}${nextUrl.hash}`);
      setLeaveConfirmOpen(true);
    };

    document.addEventListener('click', handleDocumentClick, true);
    return () => document.removeEventListener('click', handleDocumentClick, true);
  }, [isDirty, isUpdating]);

  const handleChange = useCallback(
    (key: DisplayDefaultModelUseCase, value: ModelSelectorParameterValue) => {
      setConfig(prev => ({
        ...prev,
        [key]: { model: value.model, provider: value.provider, params: value.params },
      }));
    },
    []
  );

  const handleClearExplicit = useCallback((key: DisplayDefaultModelUseCase) => {
    setConfig(prev => ({
      ...prev,
      [key]: { ...EMPTY_DEFAULT_MODEL_VALUE },
    }));
  }, []);

  const handleSave = useCallback(() => {
    updateDefaultModels(config);
  }, [config, updateDefaultModels]);

  const handleConfirmLeave = useCallback(() => {
    if (pendingHref) {
      router.push(pendingHref);
    }

    setLeaveConfirmOpen(false);
    setPendingHref(null);
  }, [pendingHref, router]);

  return (
    <div className="container max-w-7xl py-6 space-y-5">
      <div className="space-y-1.5">
        <div className="flex items-center gap-2">
          <h1 className="text-2xl font-semibold tracking-tight">
            {t('dashboard.configuration.modelSettings.title')}
          </h1>
          {isDirty ? (
            <Badge variant="warning" className="font-medium">
              {t('dashboard.configuration.modelSettings.unsavedChanges')}
            </Badge>
          ) : null}
        </div>
        <p className="max-w-3xl text-sm leading-6 text-muted-foreground">
          {t('dashboard.configuration.modelSettings.description')}
        </p>
      </div>

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {isLoading ? (
          <>
            {[1, 2, 3, 4, 5].map(i => (
              <Card key={i} className="h-full">
                <CardHeader className="pb-4">
                  <div className="flex items-center gap-3">
                    <Skeleton className="h-9 w-9 rounded-2xl" />
                    <div className="flex-1 space-y-2">
                      <Skeleton className="h-4 w-32" />
                      <Skeleton className="h-3.5 w-full" />
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="space-y-3">
                  <Skeleton className="h-4 w-24" />
                  <Skeleton className="h-10 w-full" />
                  <Skeleton className="h-3.5 w-4/5" />
                </CardContent>
              </Card>
            ))}
          </>
        ) : isError ? (
          <Card className="md:col-span-2 xl:col-span-3">
            <CardHeader>
              <div className="flex items-center gap-3">
                <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-destructive/10 shrink-0">
                  <AlertTriangle className="h-5 w-5 text-destructive" />
                </div>
                <div>
                  <CardTitle className="text-lg">{t('common.error')}</CardTitle>
                  <CardDescription className="mt-1">
                    {error instanceof Error && error.message
                      ? error.message
                      : typeof error === 'string'
                        ? error
                        : t('dashboard.configuration.modelSettings.description')}
                  </CardDescription>
                </div>
              </div>
            </CardHeader>
            <CardContent>
              <div className="flex items-center justify-end">
                <Button variant="default" onClick={() => refetch()} className="gap-2">
                  {t('common.refresh')}
                </Button>
              </div>
            </CardContent>
          </Card>
        ) : (
          USE_CASE_CARDS.map(({ key, icon: Icon, iconBg, iconColor }) => {
            const resolved = resolvedSettings[key];
            const currentValue = config[key];
            const isPendingDelete =
              resolved.source === 'explicit' && !currentValue.provider && !currentValue.model;
            const badgeKey = isPendingDelete ? 'pendingDelete' : resolved.source;
            const showClearExplicit = resolved.source === 'explicit' && !isPendingDelete;

            return (
              <Card key={key} className="relative flex h-full flex-col overflow-hidden border-border/70 shadow-sm">
                <Badge
                  variant={getSourceBadgeVariant(badgeKey)}
                  className="pointer-events-none absolute right-4 top-4 z-10 h-6 rounded-full px-2 text-[11px] font-medium"
                >
                  {t(`dashboard.configuration.modelSettings.sources.${badgeKey}`)}
                </Badge>
                <CardHeader className="space-y-0 p-4 pr-8">
                  <div className="flex items-start gap-3">
                    <div
                      className={`flex h-10 w-10 items-center justify-center rounded-2xl ${iconBg} shrink-0`}
                    >
                      <Icon className={`h-4.5 w-4.5 ${iconColor}`} />
                    </div>
                    <div className="min-w-0">
                      <CardTitle className="text-[15px] font-semibold leading-6">
                        {t(`dashboard.configuration.modelSettings.${key}.title`)}
                      </CardTitle>
                      <CardDescription className="mt-1 text-sm leading-6 text-muted-foreground">
                        {t(`dashboard.configuration.modelSettings.${key}.description`)}
                      </CardDescription>
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="flex flex-1 flex-col space-y-3 pt-0">
                  <div className="flex items-center justify-between gap-3 text-[11px] text-muted-foreground">
                    <span className="font-medium uppercase tracking-wide">
                      {t('dashboard.configuration.modelSettings.resolvedState')}
                    </span>
                    {showClearExplicit ? (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => handleClearExplicit(key)}
                        disabled={isUpdating}
                        className="h-7 px-2 text-xs text-muted-foreground hover:text-foreground"
                      >
                        {t('dashboard.configuration.modelSettings.actions.clearExplicit')}
                      </Button>
                    ) : null}
                  </div>
                  <ModelSelectorParameter
                    modelType={key}
                    value={currentValue}
                    onChange={value => handleChange(key, value)}
                    disabled={isUpdating}
                  />
                  <p className="mt-auto text-xs leading-5 text-muted-foreground">
                    {t(`dashboard.configuration.modelSettings.sourceDescriptions.${badgeKey}`)}
                  </p>
                </CardContent>
              </Card>
            );
          })
        )}
      </div>

      {!isLoading && !isError ? (
        <div className="flex justify-end pt-2">
          <Button
            onClick={handleSave}
            disabled={isUpdating || !isDirty}
            className="h-10 rounded-2xl px-4 gap-2"
          >
            {isUpdating ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
            {t('common.save')}
          </Button>
        </div>
      ) : null}

      <ConfirmDialog
        variant="warning"
        open={leaveConfirmOpen}
        onOpenChange={open => {
          setLeaveConfirmOpen(open);
          if (!open) {
            setPendingHref(null);
          }
        }}
        title={t('dashboard.configuration.modelSettings.leaveConfirm.title')}
        description={t('dashboard.configuration.modelSettings.leaveConfirm.description')}
        confirmText={t('dashboard.configuration.modelSettings.leaveConfirm.confirm') as string}
        cancelText={t('dashboard.configuration.modelSettings.leaveConfirm.cancel') as string}
        onConfirm={handleConfirmLeave}
      />
    </div>
  );
}
