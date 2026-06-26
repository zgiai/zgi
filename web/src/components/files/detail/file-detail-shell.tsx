'use client';

import { useEffect, useMemo, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';
import {
  AlertCircle,
  ArrowLeft,
  CheckCircle2,
  CircleHelp,
  Circle,
  Download,
  FileIcon,
  FileText,
  Loader2,
  MessageSquareText,
  PanelLeftClose,
  RefreshCw,
  RotateCcw,
  TriangleAlert,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import {
  FileOriginalPreviewPanel,
  type FilePreviewLocator,
} from '@/components/files/detail/file-original-preview-panel';
import {
  FileChunksPanel,
  type FileChunkLocateTarget,
} from '@/components/files/detail/file-chunks-panel';
import { FileQAPanel } from '@/components/files/detail/file-qa-panel';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { contentParseService } from '@/services/content-parse.service';
import type { ContentParseFileRouteProviderStatus } from '@/services/types/content-parse';
import type {
  FileAssetProductStatus,
  FileItem,
  FileParseProviderKey,
  FileQuestionAnswerSource,
} from '@/services/types/file';
import { useDownloadFile } from '@/hooks/use-files';
import { useFileDetail } from '@/hooks/file/use-file-detail';
import { useCreateFileProcessingRequest } from '@/hooks/file/use-file-processing-request';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { formatDate, formatFileSize } from '@/utils/format';

interface FileDetailShellProps {
  fileId: string;
}

function getProcessingStatus(file?: FileItem, status?: string): string {
  return status || file?.processing_status || 'stored_only';
}

function getProcessingBadgeVariant(status: string) {
  switch (status) {
    case 'ready':
      return 'success' as const;
    case 'parsing':
    case 'confirming':
    case 'generating':
      return 'info' as const;
    case 'parse_failed':
      return 'destructive' as const;
    case 'stored_only':
    default:
      return 'subtle' as const;
  }
}

function getDatasetReturnTo(value: string | null): string | null {
  if (!value) return null;
  if (!value.startsWith('/console/dataset/')) return null;
  if (value.startsWith('//') || value.includes('://')) return null;
  return value;
}

function metadataString(metadata: Record<string, unknown> | undefined, key: string): string {
  const value = metadata?.[key];
  if (typeof value === 'string') return value.trim();
  if (value === undefined || value === null) return '';
  return String(value).trim();
}

function parseProviderTranslationKey(provider: string) {
  return provider === 'hyperparse_api' ? 'hyperparseApi' : provider;
}

function getCurrentParseProvider(latestRequest?: {
  request_metadata?: Record<string, unknown>;
  execution_metadata?: Record<string, unknown>;
}) {
  const execution = latestRequest?.execution_metadata;
  const requested = metadataString(latestRequest?.request_metadata, 'parse_provider');
  const finalProvider =
    metadataString(execution, 'final_parse_provider') ||
    metadataString(execution, 'executed_provider_key') ||
    metadataString(execution, 'parse_provider');

  return {
    finalProvider,
    requestedProvider: requested || 'auto',
    adapter:
      metadataString(execution, 'final_parse_adapter') ||
      metadataString(execution, 'executed_adapter_name'),
    engine:
      metadataString(execution, 'final_parse_engine') ||
      metadataString(execution, 'executed_engine_name'),
  };
}

function parseProviderTranslationPath(provider: string) {
  return `upload.parseProviders.${parseProviderTranslationKey(provider)}` as never;
}

function isConfigurableParserProvider(provider: FileParseProviderKey): provider is 'mineru' | 'reducto' {
  return provider === 'mineru' || provider === 'reducto';
}

type WorkbenchStepState = 'done' | 'active' | 'attention' | 'failed' | 'blocked' | 'pending';

function getWorkbenchStepStates(
  status: string,
  pendingCount: number,
  embeddingCount: number,
  vectorStatus?: string
): Array<{ key: string; state: WorkbenchStepState; count?: number }> {
  return [
    { key: 'uploaded', state: 'done' },
    {
      key: 'parsed',
      state:
        status === 'stored_only'
          ? 'pending'
          : status === 'parsing'
            ? 'active'
            : status === 'parse_failed'
              ? 'failed'
              : 'done',
    },
    {
      key: 'quality',
      state:
        status === 'confirming'
          ? 'attention'
          : status === 'ready' || status === 'generating'
            ? 'done'
            : status === 'parse_failed'
              ? 'failed'
              : 'pending',
      count: pendingCount,
    },
    {
      key: 'chunks',
      state:
        status === 'generating'
          ? 'active'
          : status === 'ready'
            ? 'done'
            : status === 'parse_failed'
              ? 'blocked'
              : 'pending',
    },
    {
      key: 'index',
      state:
        status === 'ready' && vectorStatus === 'ready'
          ? 'done'
          : status === 'generating' || status === 'ready' || embeddingCount > 0
            ? 'active'
            : status === 'parse_failed'
              ? 'blocked'
              : 'pending',
    },
    {
      key: 'ready',
      state:
        status === 'ready' && vectorStatus === 'ready'
          ? 'done'
          : status === 'parse_failed'
            ? 'blocked'
            : 'pending',
    },
  ];
}

function getWorkbenchStepTone(state: WorkbenchStepState) {
  switch (state) {
    case 'done':
      return 'text-success';
    case 'active':
      return 'text-primary';
    case 'attention':
      return 'text-warning';
    case 'failed':
      return 'text-destructive';
    case 'blocked':
      return 'text-muted-foreground';
    case 'pending':
    default:
      return 'text-muted-foreground';
  }
}

function getWorkbenchBannerTone(status: string) {
  if (status === 'confirming') {
    return {
      icon: TriangleAlert,
      className: 'border-warning/40 bg-warning/5',
      iconClassName: 'bg-warning/10 text-warning',
      titleKey: 'detail.workbench.banners.confirming.title',
      descriptionKey: 'detail.workbench.banners.confirming.description',
    };
  }
  if (status === 'parse_failed') {
    return {
      icon: AlertCircle,
      className: 'border-destructive/40 bg-destructive/5',
      iconClassName: 'bg-destructive/10 text-destructive',
      titleKey: 'detail.workbench.banners.failed.title',
      descriptionKey: 'detail.workbench.banners.failed.description',
    };
  }
  if (status === 'ready') {
    return {
      icon: CheckCircle2,
      className: 'border-success/30 bg-success/5',
      iconClassName: 'bg-success/10 text-success',
      titleKey: 'detail.workbench.banners.ready.title',
      descriptionKey: 'detail.workbench.banners.ready.description',
    };
  }
  if (status === 'parsing' || status === 'generating') {
    return {
      icon: Loader2,
      className: 'border-primary/30 bg-primary/5',
      iconClassName: 'bg-primary/10 text-primary',
      titleKey: 'detail.workbench.banners.processing.title',
      descriptionKey: 'detail.workbench.banners.processing.description',
    };
  }
  return {
    icon: FileIcon,
    className: 'border-border bg-muted/20',
    iconClassName: 'bg-muted text-muted-foreground',
    titleKey: 'detail.workbench.banners.storedOnly.title',
    descriptionKey: 'detail.workbench.banners.storedOnly.description',
  };
}

function ProcessingWorkbenchOverview({
  status,
  pendingCount,
  chunkCount,
  embeddingCount,
  vectorStatus,
}: {
  status: string;
  pendingCount: number;
  chunkCount: number;
  embeddingCount: number;
  vectorStatus?: string;
}) {
  const t = useT('files');
  const steps = getWorkbenchStepStates(status, pendingCount, embeddingCount, vectorStatus);
  const banner = getWorkbenchBannerTone(status);
  const BannerIcon = banner.icon;

  return (
    <section className={cn('rounded-lg border px-4 py-3', banner.className)}>
      <div className="flex gap-3">
        <div
          className={cn(
            'mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg',
            banner.iconClassName
          )}
        >
          <BannerIcon
            className={cn('h-4 w-4', status === 'parsing' || status === 'generating' ? 'animate-spin' : '')}
          />
        </div>
        <div className="min-w-0 flex-1">
          <h2 className="text-sm font-semibold text-foreground">{t(banner.titleKey as never)}</h2>
          <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
            {t(banner.descriptionKey as never, {
              pending: pendingCount,
              chunks: chunkCount,
              embeddings: embeddingCount,
            })}
          </p>

          <div className="mt-3 flex min-w-0 flex-wrap items-center gap-x-0 gap-y-2">
            {steps.map((step, index) => {
              const isActive = step.state === 'active' || step.state === 'attention';
              const StepIcon =
                step.state === 'done'
                  ? CheckCircle2
                  : step.state === 'failed' || step.state === 'attention'
                    ? TriangleAlert
                    : Circle;
              return (
                <div key={step.key} className="flex items-center">
                  <div className={cn('flex items-center gap-1.5 font-medium', getWorkbenchStepTone(step.state))}>
                    <span
                      className={cn(
                        'flex h-6 w-6 items-center justify-center rounded-full border bg-background',
                        step.state === 'done' && 'border-success/30 bg-success/10',
                        isActive && 'border-current bg-current/10',
                        step.state === 'failed' && 'border-destructive/30 bg-destructive/10',
                        step.state === 'pending' && 'border-border',
                        step.state === 'blocked' && 'border-border bg-muted'
                      )}
                    >
                      <StepIcon className="h-3.5 w-3.5" />
                    </span>
                    <span className="text-xs">{t(`detail.workbench.steps.${step.key}` as never)}</span>
                  </div>
                  {index < steps.length - 1 ? (
                    <span
                      className={cn(
                        'mx-2 h-px w-8 bg-border sm:w-14',
                        step.state === 'done' && 'bg-success/50',
                        isActive && 'bg-current'
                      )}
                    />
                  ) : null}
                </div>
              );
            })}
          </div>
        </div>
      </div>
    </section>
  );
}

function ReparseButtonWithTooltip({
  latestRequest,
  onReparse,
  canReparse,
  loading,
}: {
  latestRequest?: {
    request_metadata?: Record<string, unknown>;
    execution_metadata?: Record<string, unknown>;
  };
  onReparse: () => void;
  canReparse: boolean;
  loading: boolean;
}) {
  const t = useT('files');
  const { finalProvider, requestedProvider, adapter, engine } =
    getCurrentParseProvider(latestRequest);
  const requestedLabel = t(parseProviderTranslationPath(requestedProvider || 'auto'));
  const finalLabel = finalProvider ? t(parseProviderTranslationPath(finalProvider)) : '';
  const shouldShowFinalProvider =
    Boolean(finalProvider) && finalProvider !== requestedProvider;
  const isAuto = requestedProvider === 'auto';

  if (!canReparse) return null;

  return (
    <Tooltip delayDuration={150}>
      <TooltipTrigger asChild>
        <Button
          variant="outline"
          className="h-9 gap-2 rounded-md px-3 text-sm"
          onClick={onReparse}
          disabled={loading}
        >
          {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RotateCcw className="h-4 w-4" />}
          {t('detail.reparse.action')}
        </Button>
      </TooltipTrigger>
      <TooltipContent side="bottom" align="start" className="max-w-md text-left">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-semibold text-foreground">
              {t('detail.parseMethod.title')}
            </span>
            <Badge variant="subtle" className="max-w-full truncate">
              {requestedLabel}
            </Badge>
            {shouldShowFinalProvider ? (
              <Badge variant="outline" className="max-w-full truncate">
                {t('detail.parseMethod.actualProvider', { provider: finalLabel })}
              </Badge>
            ) : null}
            {engine ? (
              <span className="text-xs text-muted-foreground">
                {t('detail.parseMethod.engine', { engine })}
              </span>
            ) : null}
          </div>
          <p className="leading-5 text-muted-foreground">
            {isAuto
              ? t('detail.parseMethod.autoDescription')
              : t('detail.parseMethod.manualDescription')}
            {adapter ? ` ${t('detail.parseMethod.adapter', { adapter })}` : ''}
          </p>
        </div>
      </TooltipContent>
    </Tooltip>
  );
}

function ReparseDialog({
  open,
  onOpenChange,
  provider,
  onProviderChange,
  onConfigureProvider,
  onConfirm,
  loading,
  providers,
  providersLoading,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  provider: FileParseProviderKey;
  onProviderChange: (provider: FileParseProviderKey) => void;
  onConfigureProvider: (provider: 'mineru' | 'reducto') => void;
  onConfirm: () => void;
  loading: boolean;
  providers: ContentParseFileRouteProviderStatus[];
  providersLoading: boolean;
}) {
  const { files: t, common } = useT();
  const hasProviders = providers.length > 0;
  const selectedProvider = providers.find(item => item.key === provider);
  const canConfirm = Boolean(selectedProvider?.selectable);
  const handleProviderChange = (value: string) => {
    const nextProvider = value as FileParseProviderKey;
    const nextItem = providers.find(item => item.key === nextProvider);
    if (!nextItem?.selectable && isConfigurableParserProvider(nextProvider)) {
      onConfigureProvider(nextProvider);
      return;
    }
    onProviderChange(nextProvider);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="overflow-hidden p-0 sm:max-w-[560px]">
        <DialogHeader className="p-8 pb-5">
          <DialogTitle className="text-2xl font-bold tracking-tight">
            {t('detail.reparse.confirmTitle')}
          </DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-6 px-8 pb-8 pt-1">
          <p className="text-base leading-7 text-muted-foreground">
            {t('detail.reparse.confirmDescription')}
          </p>
          <div className="space-y-3">
            <div className="flex items-center gap-2">
              <Label className="text-sm font-semibold">{t('detail.reparse.providerLabel')}</Label>
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    aria-label={t('detail.reparse.providerDescription')}
                    className="inline-flex size-5 items-center justify-center rounded-full text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                  >
                    <CircleHelp className="size-4" />
                  </button>
                </TooltipTrigger>
                <TooltipContent side="top" align="start" className="max-w-80 text-xs leading-5">
                  {t('detail.reparse.providerDescription')}
                </TooltipContent>
              </Tooltip>
            </div>
            <Select
              value={provider}
              onValueChange={handleProviderChange}
              disabled={providersLoading || !hasProviders}
            >
              <SelectTrigger className="min-h-14 bg-background px-4 py-3">
                <SelectValue
                  placeholder={
                    providersLoading
                      ? t('upload.parseProviderLoading')
                      : t('detail.reparse.noAvailableProvider')
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {providers.map(item => {
                  const configurable = isConfigurableParserProvider(item.key);
                  return (
                    <SelectItem
                      key={item.key}
                      value={item.key}
                      disabled={!item.selectable && !configurable}
                      className="py-2.5"
                    >
                      <div className="flex min-w-0 flex-col gap-0.5">
                        <span className="truncate">
                          {t(parseProviderTranslationPath(item.key))}
                        </span>
                        {item.selectable || !configurable ? (
                          <span className="truncate text-xs text-muted-foreground">
                            {item.selectable
                              ? t('detail.reparse.providerReady')
                              : item.reason || t('detail.reparse.providerUnavailable')}
                          </span>
                        ) : null}
                        {!item.selectable && configurable ? (
                          <span className="text-xs font-medium text-primary">
                            {t('detail.reparse.configureProvider')}
                          </span>
                        ) : null}
                      </div>
                    </SelectItem>
                  );
                })}
              </SelectContent>
            </Select>
          </div>
        </DialogBody>
        <DialogFooter className="gap-3 border-t bg-muted/30 px-8 py-5">
          <Button
            variant="ghost"
            size="xl"
            className="px-6 font-semibold"
            onClick={() => onOpenChange(false)}
          >
            {common('cancel')}
          </Button>
          <Button
            variant="destructive"
            size="xl"
            className="px-6 font-semibold"
            onClick={onConfirm}
            disabled={loading || providersLoading || !canConfirm}
          >
            {loading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
            {t('detail.reparse.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function FileDetailLoading() {
  return (
    <div className="flex h-full min-h-0 flex-col overflow-y-auto bg-bg-canvas">
      <div className="border-b bg-background px-6 py-5">
        <Skeleton className="h-9 w-32" />
        <div className="mt-5 flex items-start justify-between gap-4">
          <div className="min-w-0 flex-1 space-y-3">
            <Skeleton className="h-8 w-80 max-w-full" />
            <Skeleton className="h-4 w-96 max-w-full" />
          </div>
          <Skeleton className="h-9 w-28" />
        </div>
      </div>
      <div className="grid gap-4 p-6 md:grid-cols-3">
        <Skeleton className="h-28" />
        <Skeleton className="h-28" />
        <Skeleton className="h-28" />
      </div>
    </div>
  );
}

function FilePreviewChunksWorkbench({
  file,
  chunksEnabled,
  chunkQueryVersion,
  locateTarget,
}: {
  file: FileItem;
  chunksEnabled: boolean;
  chunkQueryVersion?: number | string | null;
  locateTarget?: FileChunkLocateTarget | null;
}) {
  const t = useT('files');
  const [originalPreviewHidden, setOriginalPreviewHidden] = useState(false);
  const [activePreviewLocator, setActivePreviewLocator] = useState<FilePreviewLocator | null>(null);
  const locateIssue = (locator: FilePreviewLocator) => {
    setOriginalPreviewHidden(false);
    setActivePreviewLocator(locator);
  };

  return (
    <div
      className={cn(
        'grid min-h-0 flex-1 bg-bg-canvas',
        originalPreviewHidden
          ? 'grid-cols-1'
          : 'xl:grid-cols-[minmax(0,1fr)_minmax(480px,0.95fr)]'
      )}
    >
      {!originalPreviewHidden ? (
        <section className="flex min-h-0 min-w-0 flex-col overflow-hidden border-r bg-background">
          <div className="flex min-h-16 shrink-0 items-center justify-between gap-3 border-b bg-background px-4 py-3">
            <span className="inline-flex rounded-full bg-muted px-4 py-2 text-sm font-semibold text-foreground">
              {t('detail.tabs.originalPreview')}
            </span>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-8 gap-1.5 rounded-md px-2.5 text-sm"
              onClick={() => setOriginalPreviewHidden(true)}
            >
              <PanelLeftClose className="h-4 w-4" />
              {t('detail.previewToggle.hideOriginal')}
            </Button>
          </div>
          <FileOriginalPreviewPanel
            file={file}
            className="h-full"
            hideHeader
            activeLocator={activePreviewLocator}
          />
        </section>
      ) : null}
      <section className="min-h-0 min-w-0 overflow-hidden">
        <FileChunksPanel
          fileId={file.id}
          enabled={chunksEnabled}
          queryVersion={chunkQueryVersion}
          className="h-full"
          originalPreviewHidden={originalPreviewHidden}
          locateTarget={locateTarget}
          onToggleOriginalPreview={() => setOriginalPreviewHidden(current => !current)}
          onLocateIssue={locateIssue}
        />
      </section>
    </div>
  );
}

export function FileDetailShell({ fileId }: FileDetailShellProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { files: t, common } = useT();
  const { hasPermission } = useAccountPermissions();
  const canDownload = hasPermission('file.download');
  const { downloadFile, isDownloading } = useDownloadFile();
  const [activeView, setActiveView] = useState<'preview' | 'qa'>('preview');
  const [chunkLocateTarget, setChunkLocateTarget] = useState<FileChunkLocateTarget | null>(null);
  const [reparseConfirmOpen, setReparseConfirmOpen] = useState(false);
  const [pendingParserConfigProvider, setPendingParserConfigProvider] =
    useState<'mineru' | 'reducto' | null>(null);
  const [selectedReparseProvider, setSelectedReparseProvider] =
    useState<FileParseProviderKey>('auto');
  const datasetReturnTo = getDatasetReturnTo(searchParams.get('returnTo'));
  const parentHref = datasetReturnTo ?? '/console/files';
  const parentLabel = datasetReturnTo ? t('detail.datasetBreadcrumb') : t('detail.fileBreadcrumb');
  const { data, isLoading, isFetching, error, refetch } = useFileDetail(fileId, {
    pollProcessingStatus: true,
  });
  const createProcessingRequest = useCreateFileProcessingRequest(fileId);

  const detail = data?.data;
  const file = detail?.file;
  const asset = detail?.asset;
  const processing = detail?.processing;
  const artifactState = detail?.artifact_state;
  const summary = processing?.summary;
  const status = getProcessingStatus(file, summary?.product_status ?? asset?.product_status);
  const vectorStatus = summary?.vector_status ?? asset?.vector_status ?? artifactState?.vector_status ?? file?.vector_status;
  const pendingCount =
    processing?.pending_confirmation_count ?? summary?.pending_confirmation_count ?? file?.pending_confirmation_count ?? 0;
  const chunkCount =
    processing?.chunk_count ?? summary?.chunk_count ?? asset?.chunk_count ?? file?.chunk_count ?? artifactState?.chunk_count ?? 0;
  const chunkQueryVersion = summary?.generation_no ?? asset?.generation_no ?? file?.generation_no ?? null;
  const embeddingCount = processing?.embedding_count ?? file?.embedding_count ?? 0;
  const chunksEnabled = status === 'ready';
  const qaEnabled = status === 'ready' && vectorStatus === 'ready' && embeddingCount > 0;
  const isFullyReady = status === 'ready' && vectorStatus === 'ready';
  const showProcessingWorkbench = !isFullyReady;
  const showHeaderRefresh = !isFullyReady;
  const canRequestProcessing =
    hasPermission('file.manage') || hasPermission('file.upload_create') || canDownload;
  const canReparse = canRequestProcessing && (status === 'ready' || status === 'parse_failed');
  const providerStatusQuery = useQuery({
    queryKey: ['content-parse', 'file-route-providers', file?.name],
    queryFn: () => contentParseService.listFileRouteProviders(file?.name ?? ''),
    enabled: canReparse && Boolean(file?.name),
    staleTime: 30_000,
  });
  const reparseProviders = useMemo(
    () => providerStatusQuery.data?.data.providers ?? [],
    [providerStatusQuery.data?.data.providers]
  );

  const statusLabel = useMemo(() => {
    switch (status as FileAssetProductStatus | string) {
      case 'parsing':
        return t('processingStatus.parsing');
      case 'confirming':
        return t('processingStatus.confirming');
      case 'generating':
        return t('processingStatus.generating');
      case 'parse_failed':
        return t('processingStatus.parse_failed');
      case 'ready':
        return t('processingStatus.ready');
      case 'stored_only':
      default:
        return t('processingStatus.stored_only');
    }
  }, [status, t]);

  const handleDownload = async () => {
    if (!file) return;
    await downloadFile(file.id, file.name);
  };
  const handleReparse = () => {
    void createProcessingRequest.mutateAsync({
      mode: 'reparse',
      target_level: 'vectorize',
      force: false,
      parse_provider: selectedReparseProvider,
    });
  };
  const handleConfigureParser = (provider: 'mineru' | 'reducto') => {
    setPendingParserConfigProvider(provider);
  };
  const handleConfirmConfigureParser = () => {
    if (!pendingParserConfigProvider) return;
    const provider = pendingParserConfigProvider;
    const returnTo = `${window.location.pathname}${window.location.search}`;
    setPendingParserConfigProvider(null);
    router.push(
      `/dashboard/settings/parsers?provider=${provider}&returnTo=${encodeURIComponent(returnTo)}`
    );
  };

  useEffect(() => {
    if (status === 'confirming') {
      setActiveView('preview');
      return;
    }
  }, [status]);

  useEffect(() => {
    if (activeView === 'qa' && !qaEnabled) {
      setActiveView('preview');
    }
  }, [activeView, qaEnabled]);

  useEffect(() => {
    setChunkLocateTarget(null);
  }, [fileId]);

  useEffect(() => {
    if (!reparseProviders.length) return;
    if (reparseProviders.some(provider => provider.key === selectedReparseProvider)) {
      return;
    }
    const firstSelectableProvider = reparseProviders.find(provider => provider.selectable);
    setSelectedReparseProvider(firstSelectableProvider?.key ?? reparseProviders[0].key);
  }, [selectedReparseProvider, reparseProviders]);

  if (isLoading) return <FileDetailLoading />;

  if (error || !file) {
    return (
      <div className="flex h-full min-h-0 flex-col bg-bg-canvas">
        <div className="border-b bg-background px-6 py-4">
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="ghost" className="gap-2" onClick={() => router.push(parentHref)}>
              <ArrowLeft className="h-4 w-4" />
              {datasetReturnTo ? t('detail.backToDataset') : t('detail.backToFiles')}
            </Button>
          </div>
        </div>
        <div className="flex flex-1 items-center justify-center p-6">
          <Alert variant="destructive" className="max-w-xl">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>{t('detail.loadErrorTitle')}</AlertTitle>
            <AlertDescription>{t('detail.loadErrorDescription')}</AlertDescription>
          </Alert>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden bg-background">
      <div className="shrink-0 border-b bg-background px-4 py-3 sm:px-6">
        <div className="flex flex-col gap-3 xl:flex-row xl:items-start xl:justify-between">
          <div className="min-w-0">
            <div className="flex min-w-0 flex-wrap items-center gap-2.5">
              <Button
                variant="ghost"
                className="h-auto gap-2 px-0 py-0 text-sm font-medium text-muted-foreground hover:bg-transparent hover:text-foreground"
                onClick={() => router.push(parentHref)}
              >
                <ArrowLeft className="h-4 w-4" />
                {parentLabel}
              </Button>
              <span className="text-lg text-muted-foreground">/</span>
              <h1 className="min-w-0 max-w-[min(720px,100%)] truncate text-xl font-semibold leading-tight text-foreground">
                {file.name}
              </h1>
              <Badge variant={getProcessingBadgeVariant(status)} className="px-2.5 py-0.5 text-xs">
                {statusLabel}
              </Badge>
            </div>
            <div className="mt-3 flex flex-wrap items-center gap-x-5 gap-y-1.5 text-sm text-muted-foreground">
              <span>{t('detail.fileType', { extension: file.extension.toUpperCase() })}</span>
              <span>{formatFileSize(file.size)}</span>
              <span>{t('detail.createdAt', { time: formatDate(file.created_at) })}</span>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <div className="flex h-9 items-center rounded-md border border-border bg-muted/30 p-0.5">
              <Button
                type="button"
                variant="ghost"
                className={cn(
                  'h-8 gap-1.5 rounded px-3 text-sm',
                  activeView === 'preview'
                    ? 'bg-background text-primary shadow-sm hover:bg-background'
                    : 'text-muted-foreground hover:text-foreground'
                )}
                onClick={() => setActiveView('preview')}
              >
                <FileText className="h-4 w-4" />
                {t('detail.tabs.preview')}
              </Button>
              <Button
                type="button"
                variant="ghost"
                className={cn(
                  'h-8 gap-1.5 rounded px-3 text-sm',
                  activeView === 'qa'
                    ? 'bg-background text-primary shadow-sm hover:bg-background'
                    : 'text-muted-foreground hover:text-foreground'
                )}
                onClick={() => setActiveView('qa')}
                disabled={!qaEnabled}
                title={!qaEnabled ? t('detail.tabHints.qaWaiting') : undefined}
              >
                <MessageSquareText className="h-4 w-4" />
                {t('detail.tabs.qa')}
              </Button>
            </div>
            {showHeaderRefresh ? (
              <Button
                variant="outline"
                className="h-9 gap-2 rounded-md px-3 text-sm"
                onClick={() => void refetch()}
                disabled={isFetching}
              >
                {isFetching ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <RefreshCw className="h-4 w-4" />
                )}
                {common('refresh')}
              </Button>
            ) : null}
            <ReparseButtonWithTooltip
              latestRequest={processing?.latest_request}
              canReparse={canReparse}
              loading={createProcessingRequest.isPending}
              onReparse={() => setReparseConfirmOpen(true)}
            />
            {canDownload ? (
              <Button
                variant="outline"
                className="h-9 gap-2 rounded-md px-3 text-sm"
                onClick={handleDownload}
                disabled={isDownloading}
              >
                {isDownloading ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Download className="h-4 w-4" />
                )}
                {t('detail.downloadOriginal')}
              </Button>
            ) : null}
          </div>
        </div>
      </div>

      {detail.error?.message || file.last_error_message ? (
        <div className="shrink-0 px-4 pt-4 sm:px-6">
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>{t('detail.processingError')}</AlertTitle>
            <AlertDescription>
              <div>{detail.error?.message || file.last_error_message}</div>
              {status === 'parse_failed' ? (
                <div className="mt-3 flex flex-wrap gap-2">
                  {canReparse ? (
                    <Button
                      size="sm"
                      variant="destructive"
                      className="gap-2"
                      onClick={() => setReparseConfirmOpen(true)}
                    >
                      <RefreshCw className="h-4 w-4" />
                      {t('detail.reparse.action')}
                    </Button>
                  ) : null}
                  <Button
                    size="sm"
                    variant="outline"
                    disabled
                    title={t('detail.failure.storeOnlyUnavailable')}
                  >
                    {t('detail.failure.storeOnly')}
                  </Button>
                </div>
              ) : null}
            </AlertDescription>
          </Alert>
        </div>
      ) : null}

      {showProcessingWorkbench ? (
        <div className="shrink-0 border-b px-4 py-3 sm:px-6">
          <ProcessingWorkbenchOverview
            status={status}
            pendingCount={pendingCount}
            chunkCount={chunkCount}
            embeddingCount={embeddingCount}
            vectorStatus={vectorStatus}
          />
        </div>
      ) : null}

      <main className="min-h-0 min-w-0 flex-1 overflow-hidden">
        <section
          className={cn(
            'h-full min-h-0 flex-col',
            activeView === 'preview' ? 'flex' : 'hidden'
          )}
          aria-hidden={activeView !== 'preview'}
        >
          <FilePreviewChunksWorkbench
            file={file}
            chunksEnabled={chunksEnabled}
            chunkQueryVersion={chunkQueryVersion}
            locateTarget={chunkLocateTarget}
          />
        </section>
        <section
          className={cn(
            'h-full min-h-0 overflow-y-auto bg-bg-canvas p-4 sm:p-6',
            activeView === 'qa' ? 'block' : 'hidden'
          )}
          aria-hidden={activeView !== 'qa'}
        >
          <FileQAPanel
            fileId={file.id}
            artifactState={artifactState}
            processing={processing}
            vectorStatus={vectorStatus}
            enabled={qaEnabled}
            onLocateSource={(source: FileQuestionAnswerSource) => {
              setChunkLocateTarget({
                chunkId: source.primary_chunk_id,
                requestId: Date.now(),
              });
              setActiveView('preview');
            }}
          />
        </section>
      </main>

      <ReparseDialog
        open={reparseConfirmOpen}
        onOpenChange={setReparseConfirmOpen}
        provider={selectedReparseProvider}
        onProviderChange={setSelectedReparseProvider}
        onConfigureProvider={handleConfigureParser}
        providers={reparseProviders}
        providersLoading={providerStatusQuery.isLoading || providerStatusQuery.isFetching}
        onConfirm={() => {
          handleReparse();
          setReparseConfirmOpen(false);
        }}
        loading={createProcessingRequest.isPending}
      />
      <ConfirmDialog
        variant="default"
        open={Boolean(pendingParserConfigProvider)}
        onOpenChange={open => {
          if (!open) setPendingParserConfigProvider(null);
        }}
        title={t('detail.reparse.configureProviderConfirmTitle')}
        description={t('detail.reparse.configureProviderConfirmDescription')}
        confirmText={t('detail.reparse.configureProviderConfirmAction')}
        cancelText={common('cancel')}
        onConfirm={handleConfirmConfigureParser}
      />
    </div>
  );
}
