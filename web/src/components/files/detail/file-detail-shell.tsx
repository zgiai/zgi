'use client';

import { useEffect, useMemo, useState, type ComponentType } from 'react';
import { useRouter } from 'next/navigation';
import {
  AlertCircle,
  ArrowLeft,
  CalendarDays,
  CheckCircle2,
  ClipboardCheck,
  Download,
  Eye,
  FileIcon,
  HardDrive,
  Loader2,
  RefreshCw,
  ScissorsLineDashed,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Progress } from '@/components/ui/progress';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { FilePreviewDialog } from '@/components/files/file-preview-dialog';
import { FileOriginalPreviewPanel } from '@/components/files/detail/file-original-preview-panel';
import { FileParseReviewPanel } from '@/components/files/detail/file-parse-review-panel';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { FileAssetProductStatus, FileAssetVectorStatus, FileItem } from '@/services/types/file';
import { useDownloadFile } from '@/hooks/use-files';
import { useFileDetail } from '@/hooks/file/use-file-detail';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { formatDate, formatFileSize } from '@/utils/format';
import { isOriginalPreviewSupported } from '@/utils/file-helpers';

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

function getVectorBadgeVariant(status?: string) {
  switch (status) {
    case 'ready':
      return 'success' as const;
    case 'indexing':
      return 'info' as const;
    case 'failed':
      return 'destructive' as const;
    case 'none':
    default:
      return 'subtle' as const;
  }
}

function DetailField({ label, value }: { label: string; value?: string | number | null }) {
  return (
    <div className="min-w-0 rounded-md border border-border/70 bg-background px-3 py-2">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 truncate text-sm font-medium text-foreground">
        {value === undefined || value === null || value === '' ? '-' : value}
      </div>
    </div>
  );
}

function DetailStat({
  label,
  value,
  icon: Icon,
}: {
  label: string;
  value: string | number;
  icon: ComponentType<{ className?: string }>;
}) {
  return (
    <div className="rounded-md border border-border/70 bg-background p-3">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <Icon className="h-3.5 w-3.5" />
        <span>{label}</span>
      </div>
      <div className="mt-2 text-lg font-semibold text-foreground">{value}</div>
    </div>
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

export function FileDetailShell({ fileId }: FileDetailShellProps) {
  const router = useRouter();
  const { files: t, common } = useT();
  const { hasPermission } = useAccountPermissions();
  const canDownload = hasPermission('file.download');
  const { downloadFile, isDownloading } = useDownloadFile();
  const [previewFile, setPreviewFile] = useState<FileItem | null>(null);
  const [activeTab, setActiveTab] = useState('overview');
  const { data, isLoading, isFetching, error, refetch } = useFileDetail(fileId, {
    pollProcessingStatus: true,
  });

  const detail = data?.data;
  const file = detail?.file;
  const processing = detail?.processing;
  const artifactState = detail?.artifact_state;
  const summary = processing?.summary;
  const status = getProcessingStatus(file, summary?.product_status);
  const progress = summary?.processing_progress ?? file?.processing_progress ?? 0;
  const vectorStatus = artifactState?.vector_status ?? file?.vector_status;
  const pendingCount = processing?.pending_confirmation_count ?? file?.pending_confirmation_count ?? 0;
  const chunkCount = processing?.chunk_count ?? file?.chunk_count ?? artifactState?.chunk_count ?? 0;
  const embeddingCount = processing?.embedding_count ?? file?.embedding_count ?? 0;
  const hasPreview = file ? isOriginalPreviewSupported(file.extension, file.mime_type) : false;
  const parseReviewEnabled = status !== 'stored_only' && status !== 'parsing';

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

  const vectorStatusLabel = useMemo(() => {
    switch (vectorStatus as FileAssetVectorStatus | string | undefined) {
      case 'indexing':
        return t('detail.vectorStatus.indexing');
      case 'ready':
        return t('detail.vectorStatus.ready');
      case 'failed':
        return t('detail.vectorStatus.failed');
      case 'none':
      default:
        return t('detail.vectorStatus.none');
    }
  }, [t, vectorStatus]);

  const currentView = useMemo(() => {
    switch (status as FileAssetProductStatus | string) {
      case 'parsing':
        return {
          title: t('detail.views.processing.title'),
          description: t('detail.views.processing.description'),
          icon: Loader2,
          spinning: true,
        };
      case 'confirming':
        return {
          title: t('detail.views.confirming.title'),
          description: t('detail.views.confirming.description'),
          icon: ClipboardCheck,
          spinning: false,
        };
      case 'generating':
        return {
          title: t('detail.views.generating.title'),
          description: t('detail.views.generating.description'),
          icon: Loader2,
          spinning: true,
        };
      case 'ready':
        return {
          title: t('detail.views.ready.title'),
          description: t('detail.views.ready.description'),
          icon: CheckCircle2,
          spinning: false,
        };
      case 'parse_failed':
        return {
          title: t('detail.views.failed.title'),
          description: t('detail.views.failed.description'),
          icon: AlertCircle,
          spinning: false,
        };
      case 'stored_only':
      default:
        return {
          title: t('detail.views.storedOnly.title'),
          description: t('detail.views.storedOnly.description'),
          icon: ScissorsLineDashed,
          spinning: false,
        };
    }
  }, [status, t]);

  const handleDownload = async () => {
    if (!file) return;
    await downloadFile(file.id, file.name);
  };
  const CurrentViewIcon = currentView.icon;

  useEffect(() => {
    if (status === 'confirming') {
      setActiveTab('parse-review');
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
    <div className="flex h-full min-h-0 flex-col overflow-y-auto bg-bg-canvas">
      <div className="border-b bg-background px-4 py-4 sm:px-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Button variant="ghost" className="gap-2 px-2" onClick={() => router.push('/console/files')}>
            <ArrowLeft className="h-4 w-4" />
            {t('detail.backToFiles')}
          </Button>
          <div className="flex flex-wrap items-center gap-2">
            <Button
              variant="outline"
              className="gap-2"
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
            {hasPreview ? (
              <Button variant="outline" className="gap-2" onClick={() => setPreviewFile(file)}>
                <Eye className="h-4 w-4" />
                {t('detail.previewOriginal')}
              </Button>
            ) : null}
            {canDownload ? (
              <Button className="gap-2" onClick={handleDownload} disabled={isDownloading}>
                {isDownloading ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Download className="h-4 w-4" />
                )}
                {t('actions.downloadFile')}
              </Button>
            ) : null}
          </div>
        </div>

        <div className="mt-5 flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0 flex items-start gap-4">
            <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
              <FileIcon className="h-6 w-6" />
            </div>
            <div className="min-w-0">
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <h1 className="min-w-0 truncate text-2xl font-semibold text-foreground">
                  {file.name}
                </h1>
                <Badge variant="outline">{file.extension}</Badge>
                <Badge variant={getProcessingBadgeVariant(status)}>{statusLabel}</Badge>
              </div>
              <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground">
                <span>{formatFileSize(file.size)}</span>
                <span>{file.mime_type || '-'}</span>
                <span>{t('detail.createdAt', { time: formatDate(file.created_at) })}</span>
              </div>
            </div>
          </div>

          <div className="min-w-[240px] rounded-md border border-border bg-background p-3">
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm font-medium text-foreground">{t('detail.processing')}</span>
              <span className="text-sm text-muted-foreground">{progress}%</span>
            </div>
            <Progress className="mt-3 h-2" value={progress} />
            <div className="mt-3 flex flex-wrap gap-2">
              <Badge variant={getVectorBadgeVariant(vectorStatus)}>{vectorStatusLabel}</Badge>
              {summary?.processing_stage ? (
                <Badge variant="outline">{summary.processing_stage}</Badge>
              ) : null}
            </div>
          </div>
        </div>
      </div>

      {detail.error?.message || file.last_error_message ? (
        <div className="px-4 pt-4 sm:px-6">
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>{t('detail.processingError')}</AlertTitle>
            <AlertDescription>{detail.error?.message || file.last_error_message}</AlertDescription>
          </Alert>
        </div>
      ) : null}

      <div className="p-4 sm:p-6">
        <Tabs value={activeTab} onValueChange={setActiveTab} className="min-w-0">
          <TabsList className="flex h-auto w-full justify-start overflow-x-auto rounded-md sm:inline-flex sm:w-auto">
            <TabsTrigger value="overview">{t('detail.tabs.overview')}</TabsTrigger>
            <TabsTrigger value="original">{t('detail.tabs.originalPreview')}</TabsTrigger>
            <TabsTrigger value="parse-review" disabled={!parseReviewEnabled}>
              {t('detail.tabs.parseReview')}
            </TabsTrigger>
            <TabsTrigger value="chunks" disabled>
              {t('detail.tabs.chunks')}
            </TabsTrigger>
            <TabsTrigger value="index" disabled>
              {t('detail.tabs.index')}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="overview" className="mt-4">
            <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
              <div className="min-w-0 space-y-4">
                <section className="rounded-md border border-border bg-background p-4">
                  <div className="mb-4 flex items-center justify-between gap-3">
                    <h2 className="text-base font-semibold text-foreground">
                      {t('detail.basicInfo')}
                    </h2>
                  </div>
                  <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                    <DetailField label={t('detail.fileId')} value={file.id} />
                    <DetailField
                      label={t('detail.assetId')}
                      value={detail.asset?.id || file.asset_id}
                    />
                    <DetailField label={t('detail.storageType')} value={file.storage_type} />
                    <DetailField label={t('detail.workspaceId')} value={file.workspace_id} />
                    <DetailField label={t('detail.createdBy')} value={file.created_by} />
                    <DetailField label={t('detail.generationNo')} value={summary?.generation_no} />
                  </div>
                </section>

                <section className="rounded-md border border-border bg-background p-4">
                  <h2 className="text-base font-semibold text-foreground">
                    {t('detail.nextViews')}
                  </h2>
                  <div className="mt-4 flex gap-3 rounded-md border border-dashed border-border bg-muted/30 p-4">
                    <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-background text-muted-foreground">
                      <CurrentViewIcon
                        className={cn('h-5 w-5', currentView.spinning && 'animate-spin')}
                      />
                    </div>
                    <div className="min-w-0">
                      <div className="text-sm font-medium text-foreground">
                        {currentView.title}
                      </div>
                      <p className="mt-1 text-sm leading-6 text-muted-foreground">
                        {currentView.description}
                      </p>
                    </div>
                  </div>
                </section>
              </div>

              <aside className="space-y-4">
                <section className="rounded-md border border-border bg-background p-4">
                  <h2 className="text-base font-semibold text-foreground">
                    {t('detail.processingSummary')}
                  </h2>
                  <div className="mt-4 grid gap-3">
                    <DetailStat
                      label={t('detail.pendingConfirmationCount')}
                      value={pendingCount}
                      icon={AlertCircle}
                    />
                    <DetailStat label={t('detail.chunkCount')} value={chunkCount} icon={FileIcon} />
                    <DetailStat
                      label={t('detail.embeddingCount')}
                      value={embeddingCount}
                      icon={HardDrive}
                    />
                    <DetailStat
                      label={t('detail.createdDate')}
                      value={formatDate(file.created_at, 'YYYY-MM-DD')}
                      icon={CalendarDays}
                    />
                  </div>
                </section>

                <section className="rounded-md border border-border bg-background p-4">
                  <h2 className="text-base font-semibold text-foreground">
                    {t('detail.indexInfo')}
                  </h2>
                  <div className="mt-4 grid gap-3">
                    <DetailField
                      label={t('detail.embeddingProvider')}
                      value={artifactState?.embedding_provider}
                    />
                    <DetailField
                      label={t('detail.embeddingModel')}
                      value={artifactState?.embedding_model}
                    />
                    <DetailField
                      label={t('detail.embeddingDimension')}
                      value={artifactState?.embedding_dimension}
                    />
                  </div>
                </section>
              </aside>
            </div>
          </TabsContent>

          <TabsContent value="original" className="mt-4">
            <FileOriginalPreviewPanel
              file={file}
              onDownload={canDownload ? () => void handleDownload() : undefined}
              isDownloading={isDownloading}
            />
          </TabsContent>

          <TabsContent value="parse-review" className="mt-4">
            <FileParseReviewPanel fileId={file.id} enabled={parseReviewEnabled} />
          </TabsContent>
        </Tabs>
      </div>

      <FilePreviewDialog
        open={Boolean(previewFile)}
        onOpenChange={open => {
          if (!open) setPreviewFile(null);
        }}
        file={previewFile}
        onDownload={() => {
          void handleDownload();
        }}
        isDownloading={isDownloading}
      />
    </div>
  );
}
