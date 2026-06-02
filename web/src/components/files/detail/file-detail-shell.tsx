'use client';

import { useEffect, useMemo, useState } from 'react';
import { useRouter } from 'next/navigation';
import {
  AlertCircle,
  ArrowLeft,
  CheckCircle2,
  Circle,
  Download,
  FileDown,
  FileIcon,
  FileText,
  Layers3,
  Loader2,
  MessageSquareText,
  RefreshCw,
  Sparkles,
  TriangleAlert,
} from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { FileOriginalPreviewPanel } from '@/components/files/detail/file-original-preview-panel';
import { FileParseReviewPanel } from '@/components/files/detail/file-parse-review-panel';
import { FileChunksPanel } from '@/components/files/detail/file-chunks-panel';
import { FileQAPanel } from '@/components/files/detail/file-qa-panel';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { fileManageService } from '@/services/file-manage.service';
import type {
  FileAssetProductStatus,
  FileItem,
  FileParsePreviewElement,
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
    <section className={cn('rounded-lg border px-5 py-4', banner.className)}>
      <div className="flex gap-4">
        <div
          className={cn(
            'mt-1 flex h-11 w-11 shrink-0 items-center justify-center rounded-lg',
            banner.iconClassName
          )}
        >
          <BannerIcon className={cn('h-5 w-5', status === 'parsing' || status === 'generating' ? 'animate-spin' : '')} />
        </div>
        <div className="min-w-0 flex-1">
          <h2 className="text-base font-semibold text-foreground">{t(banner.titleKey as never)}</h2>
          <p className="mt-1 text-sm leading-6 text-muted-foreground">
            {t(banner.descriptionKey as never, {
              pending: pendingCount,
              chunks: chunkCount,
              embeddings: embeddingCount,
            })}
          </p>

          <div className="mt-5 flex min-w-0 flex-wrap items-center gap-x-0 gap-y-3">
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
                  <div className={cn('flex items-center gap-2 font-medium', getWorkbenchStepTone(step.state))}>
                    <span
                      className={cn(
                        'flex h-7 w-7 items-center justify-center rounded-full border bg-background',
                        step.state === 'done' && 'border-success/30 bg-success/10',
                        isActive && 'border-current bg-current/10',
                        step.state === 'failed' && 'border-destructive/30 bg-destructive/10',
                        step.state === 'pending' && 'border-border',
                        step.state === 'blocked' && 'border-border bg-muted'
                      )}
                    >
                      <StepIcon className="h-4 w-4" />
                    </span>
                    <span className="text-sm">{t(`detail.workbench.steps.${step.key}` as never)}</span>
                  </div>
                  {index < steps.length - 1 ? (
                    <span
                      className={cn(
                        'mx-3 h-px w-12 bg-border sm:w-20',
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

function buildParsedContent(elements: FileParsePreviewElement[]) {
  return elements
    .slice()
    .sort((a, b) => a.ordinal - b.ordinal)
    .map(element => {
      const title = [
        element.page ? `Page ${element.page}` : null,
        element.type,
        element.subtype,
      ]
        .filter(Boolean)
        .join(' / ');
      return `## ${title}\n\n${element.content || ''}`;
    })
    .join('\n\n');
}

function triggerTextDownload(filename: string, content: string) {
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
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

export function FileDetailShell({ fileId }: FileDetailShellProps) {
  const router = useRouter();
  const { files: t, common } = useT();
  const { hasPermission } = useAccountPermissions();
  const canDownload = hasPermission('file.download');
  const { downloadFile, isDownloading } = useDownloadFile();
  const [activeTab, setActiveTab] = useState('preview');
  const [reparseConfirmOpen, setReparseConfirmOpen] = useState(false);
  const [isExportingParsed, setIsExportingParsed] = useState(false);
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
  const embeddingCount = processing?.embedding_count ?? file?.embedding_count ?? 0;
  const parseReviewEnabled = status !== 'stored_only' && status !== 'parsing';
  const chunksEnabled = status === 'ready';
  const qaEnabled = status === 'ready' && vectorStatus === 'ready' && embeddingCount > 0;
  const parsedExportEnabled = parseReviewEnabled && status !== 'parse_failed';
  const canRequestProcessing =
    hasPermission('file.manage') || hasPermission('file.upload_create') || canDownload;
  const canReparse = canRequestProcessing && (status === 'ready' || status === 'parse_failed');

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
  const handleExportParsedContent = async () => {
    if (!file || !parsedExportEnabled) return;
    setIsExportingParsed(true);
    try {
      const response = await fileManageService.getParsePreview(file.id);
      const content = buildParsedContent(response.data.elements);
      const baseName = file.name.replace(/\.[^.]+$/, '') || file.name;
      triggerTextDownload(`${baseName}-parsed-content.txt`, content);
      toast.success(t('detail.exportParsedContentSuccess'));
    } catch (exportError) {
      toast.error(
        exportError instanceof Error
          ? exportError.message
          : t('detail.exportParsedContentFailed')
      );
    } finally {
      setIsExportingParsed(false);
    }
  };
  const handleReparse = () => {
    void createProcessingRequest.mutateAsync({
      mode: 'reparse',
      target_level: 'vectorize',
      force: false,
    });
  };

  useEffect(() => {
    if (status === 'confirming') {
      setActiveTab('preview');
      return;
    }
  }, [status]);

  if (isLoading) return <FileDetailLoading />;

  if (error || !file) {
    return (
      <div className="flex h-full min-h-0 flex-col bg-bg-canvas">
        <div className="border-b bg-background px-6 py-4">
          <Button variant="ghost" className="gap-2" onClick={() => router.push('/console/files')}>
            <ArrowLeft className="h-4 w-4" />
            {t('detail.backToFiles')}
          </Button>
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
    <div className="flex h-full min-h-0 flex-col overflow-y-auto bg-background">
      <div className="border-b bg-background px-4 py-5 sm:px-6">
        <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
          <div className="min-w-0">
            <div className="flex min-w-0 flex-wrap items-center gap-3">
              <Button
                variant="ghost"
                className="h-auto gap-2 px-0 py-0 text-base font-medium text-muted-foreground hover:bg-transparent hover:text-foreground"
                onClick={() => router.push('/console/files')}
              >
                <ArrowLeft className="h-5 w-5" />
                {t('detail.fileBreadcrumb')}
              </Button>
              <span className="text-xl text-muted-foreground">/</span>
              <h1 className="min-w-0 max-w-[min(720px,100%)] truncate text-2xl font-semibold leading-tight text-foreground">
                {file.name}
              </h1>
              <Badge variant={getProcessingBadgeVariant(status)} className="px-3 py-1 text-sm">
                {statusLabel}
              </Badge>
            </div>
            <div className="mt-4 flex flex-wrap items-center gap-x-6 gap-y-2 text-sm text-muted-foreground">
              <span>{t('detail.fileType', { extension: file.extension.toUpperCase() })}</span>
              <span>{formatFileSize(file.size)}</span>
              <span>{t('detail.createdAt', { time: formatDate(file.created_at) })}</span>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Button
              variant="outline"
              className="h-11 gap-2 rounded-lg px-4"
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
            {canDownload ? (
              <Button
                variant="outline"
                className="h-11 gap-2 rounded-lg px-4"
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
            <Button
              variant="outline"
              className="h-11 gap-2 rounded-lg px-4"
              onClick={() => void handleExportParsedContent()}
              disabled={!parsedExportEnabled || isExportingParsed}
              title={!parsedExportEnabled ? t('detail.exportParsedContentUnavailable') : undefined}
            >
              {isExportingParsed ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <FileDown className="h-4 w-4" />
              )}
              {t('detail.exportParsedContent')}
            </Button>
            {canReparse ? (
              <Button
                variant="outline"
                className="h-11 gap-2 rounded-lg px-4"
                onClick={() => setReparseConfirmOpen(true)}
                disabled={createProcessingRequest.isPending}
              >
                {createProcessingRequest.isPending ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <RefreshCw className="h-4 w-4" />
                )}
                {t('detail.reparse.action')}
              </Button>
            ) : null}
          </div>
        </div>
      </div>

      {detail.error?.message || file.last_error_message ? (
        <div className="px-4 pt-4 sm:px-6">
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

      <div className="border-b px-4 py-4 sm:px-6">
        <ProcessingWorkbenchOverview
          status={status}
          pendingCount={pendingCount}
          chunkCount={chunkCount}
          embeddingCount={embeddingCount}
          vectorStatus={vectorStatus}
        />
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab} className="min-w-0">
        <TabsList className="grid h-auto w-full grid-cols-3 overflow-hidden rounded-none border-x-0 border-t-0 bg-background p-0 text-foreground">
          <TabsTrigger
            value="preview"
            className="min-h-[84px] justify-start gap-3 rounded-none border-0 border-r border-border px-5 text-left shadow-none data-[state=active]:border-x-0 data-[state=active]:border-b-2 data-[state=active]:border-b-primary data-[state=active]:bg-background data-[state=active]:shadow-none"
          >
            <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-warning/10 text-warning">
              <FileText className="h-5 w-5" />
            </span>
            <span className="min-w-0">
              <span className="block text-base font-semibold">{t('detail.tabs.preview')}</span>
            </span>
          </TabsTrigger>
          <TabsTrigger
            value="chunks"
            disabled={!chunksEnabled}
            className="min-h-[84px] justify-start gap-3 rounded-none border-0 border-r border-border px-5 text-left shadow-none data-[state=active]:border-x-0 data-[state=active]:border-b-2 data-[state=active]:border-b-primary data-[state=active]:bg-background data-[state=active]:shadow-none"
          >
            <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <Layers3 className="h-5 w-5" />
            </span>
            <span className="min-w-0">
              <span className="block text-base font-semibold">{t('detail.tabs.chunks')}</span>
              <span className="mt-1 block truncate text-sm font-normal text-muted-foreground">
                {chunksEnabled
                  ? t('detail.tabHints.chunksReady', { count: chunkCount })
                  : t('detail.tabHints.chunksWaiting')}
              </span>
            </span>
          </TabsTrigger>
          <TabsTrigger
            value="qa"
            disabled={!qaEnabled}
            className="min-h-[84px] justify-start gap-3 rounded-none border-0 px-5 text-left shadow-none data-[state=active]:border-x-0 data-[state=active]:border-b-2 data-[state=active]:border-b-primary data-[state=active]:bg-background data-[state=active]:shadow-none"
          >
            <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <MessageSquareText className="h-5 w-5" />
            </span>
            <span className="min-w-0">
              <span className="block text-base font-semibold">{t('detail.tabs.qa')}</span>
              <span className="mt-1 block truncate text-sm font-normal text-muted-foreground">
                {qaEnabled ? t('detail.tabHints.qaReady') : t('detail.tabHints.qaWaiting')}
              </span>
            </span>
          </TabsTrigger>
        </TabsList>

        <TabsContent value="preview" className="mt-0">
          <section>
            <div className="border-b px-4 py-5 sm:px-6">
              <h2 className="text-xl font-semibold text-foreground">{t('detail.tabs.preview')}</h2>
              <p className="mt-2 text-sm text-muted-foreground">
                {t('detail.previewWorkspaceDescription')}
              </p>
            </div>
            <div className="grid min-h-[620px] xl:grid-cols-[minmax(0,1fr)_minmax(420px,0.95fr)]">
              <FileOriginalPreviewPanel
                file={file}
                isDownloading={isDownloading}
              />
              <div className="min-w-0 border-t bg-background xl:border-l xl:border-t-0">
                <div className="flex min-h-16 flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
                  <Badge variant="subtle" className="px-4 py-2 text-sm font-semibold text-foreground">
                    {t('detail.workbench.steps.parsed')}
                  </Badge>
                  {canReparse ? (
                    <Button
                      variant="outline"
                      className="h-10 gap-2 rounded-lg"
                      onClick={() => setReparseConfirmOpen(true)}
                      disabled={createProcessingRequest.isPending}
                    >
                      {createProcessingRequest.isPending ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <Sparkles className="h-4 w-4" />
                      )}
                      {t('detail.reparse.action')}
                    </Button>
                  ) : null}
                </div>
                <div className="max-h-[calc(100vh-430px)] min-h-[560px] overflow-y-auto p-4">
                  <FileParseReviewPanel fileId={file.id} enabled={parseReviewEnabled} compact />
                </div>
              </div>
            </div>
          </section>
        </TabsContent>

        <TabsContent value="chunks" className="mt-0 bg-bg-canvas p-4 sm:p-6">
          <FileChunksPanel fileId={file.id} enabled={chunksEnabled} />
        </TabsContent>

        <TabsContent value="qa" className="mt-0 bg-bg-canvas p-4 sm:p-6">
          <FileQAPanel
            fileId={file.id}
            artifactState={artifactState}
            processing={processing}
            vectorStatus={vectorStatus}
            enabled={qaEnabled}
          />
        </TabsContent>
      </Tabs>

      <ConfirmDialog
        open={reparseConfirmOpen}
        onOpenChange={setReparseConfirmOpen}
        title={t('detail.reparse.confirmTitle')}
        description={t('detail.reparse.confirmDescription')}
        confirmText={t('detail.reparse.confirm')}
        cancelText={common('cancel')}
        onConfirm={handleReparse}
        loading={createProcessingRequest.isPending}
        variant="warning"
      />
    </div>
  );
}
