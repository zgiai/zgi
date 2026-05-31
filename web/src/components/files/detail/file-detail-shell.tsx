'use client';

import { useEffect, useMemo, useState } from 'react';
import { useRouter } from 'next/navigation';
import {
  AlertCircle,
  ArrowLeft,
  Download,
  Eye,
  FileIcon,
  FileText,
  Layers3,
  Loader2,
  MessageSquareText,
  RefreshCw,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Progress } from '@/components/ui/progress';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { FilePreviewDialog } from '@/components/files/file-preview-dialog';
import { FileOriginalPreviewPanel } from '@/components/files/detail/file-original-preview-panel';
import { FileParseReviewPanel } from '@/components/files/detail/file-parse-review-panel';
import { FileChunksPanel } from '@/components/files/detail/file-chunks-panel';
import { FileIndexInfoPanel } from '@/components/files/detail/file-index-info-panel';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { FileAssetProductStatus, FileAssetVectorStatus, FileItem } from '@/services/types/file';
import { useDownloadFile } from '@/hooks/use-files';
import { useFileDetail } from '@/hooks/file/use-file-detail';
import { useCreateFileProcessingRequest } from '@/hooks/file/use-file-processing-request';
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

function getWorkbenchStepStates(status: string, pendingCount: number, embeddingCount: number) {
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
        status === 'ready'
          ? 'done'
          : status === 'generating' || embeddingCount > 0
            ? 'active'
            : status === 'parse_failed'
              ? 'blocked'
              : 'pending',
    },
    {
      key: 'ready',
      state: status === 'ready' ? 'done' : status === 'parse_failed' ? 'blocked' : 'pending',
    },
  ];
}

function getWorkbenchStepTone(state: string) {
  switch (state) {
    case 'done':
      return 'border-success/30 bg-success/10 text-success';
    case 'active':
      return 'border-primary/30 bg-primary/10 text-primary';
    case 'attention':
      return 'border-warning/40 bg-warning/10 text-warning';
    case 'failed':
      return 'border-destructive/30 bg-destructive/10 text-destructive';
    case 'blocked':
      return 'border-border bg-muted text-muted-foreground';
    case 'pending':
    default:
      return 'border-border bg-background text-muted-foreground';
  }
}

function ProcessingWorkbenchOverview({
  status,
  progress,
  pendingCount,
  chunkCount,
  embeddingCount,
}: {
  status: string;
  progress: number;
  pendingCount: number;
  chunkCount: number;
  embeddingCount: number;
}) {
  const t = useT('files');
  const steps = getWorkbenchStepStates(status, pendingCount, embeddingCount);

  return (
    <section className="rounded-md border border-border bg-background p-4">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <h2 className="text-base font-semibold text-foreground">
            {t('detail.workbench.title')}
          </h2>
          <p className="mt-1 text-sm leading-6 text-muted-foreground">
            {t('detail.workbench.description', {
              pending: pendingCount,
              chunks: chunkCount,
              embeddings: embeddingCount,
            })}
          </p>
        </div>
        <div className="min-w-[220px]">
          <div className="flex items-center justify-between text-xs text-muted-foreground">
            <span>{t('detail.processing')}</span>
            <span>{progress}%</span>
          </div>
          <Progress className="mt-2 h-2" value={progress} />
        </div>
      </div>

      <div className="mt-4 grid gap-2 sm:grid-cols-3 xl:grid-cols-6">
        {steps.map(step => (
          <div
            key={step.key}
            className={cn(
              'min-h-[72px] rounded-md border px-3 py-2',
              getWorkbenchStepTone(step.state)
            )}
          >
            <div className="text-xs font-medium">
              {t(`detail.workbench.steps.${step.key}` as never)}
            </div>
            <div className="mt-2 text-[11px] text-current/75">
              {step.key === 'quality' && pendingCount > 0
                ? t('detail.workbench.pendingHint', { count: pendingCount })
                : t(`detail.workbench.stepStates.${step.state}` as never)}
            </div>
          </div>
        ))}
      </div>
    </section>
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
  const [activeTab, setActiveTab] = useState('preview');
  const [reparseConfirmOpen, setReparseConfirmOpen] = useState(false);
  const { data, isLoading, isFetching, error, refetch } = useFileDetail(fileId, {
    pollProcessingStatus: true,
  });
  const createProcessingRequest = useCreateFileProcessingRequest(fileId);

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
  const chunksEnabled = status === 'ready';
  const qaEnabled = status === 'ready' || status === 'generating' || embeddingCount > 0;
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

  const handleDownload = async () => {
    if (!file) return;
    await downloadFile(file.id, file.name);
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
            {canReparse ? (
              <Button
                variant="destructive"
                className="gap-2"
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

      <div className="space-y-4 p-4 sm:p-6">
        <ProcessingWorkbenchOverview
          status={status}
          progress={progress}
          pendingCount={pendingCount}
          chunkCount={chunkCount}
          embeddingCount={embeddingCount}
        />

        <Tabs value={activeTab} onValueChange={setActiveTab} className="min-w-0">
          <TabsList className="flex h-auto w-full justify-start overflow-x-auto rounded-md sm:inline-flex sm:w-auto">
            <TabsTrigger value="preview" className="gap-2">
              <FileText className="h-4 w-4" />
              {t('detail.tabs.preview')}
            </TabsTrigger>
            <TabsTrigger value="chunks" disabled={!chunksEnabled}>
              <Layers3 className="mr-2 h-4 w-4" />
              {t('detail.tabs.chunks')}
              {chunkCount > 0 ? (
                <Badge variant="subtle" className="ml-2 px-1.5 py-0 text-[10px]">
                  {chunkCount}
                </Badge>
              ) : null}
            </TabsTrigger>
            <TabsTrigger value="qa" disabled={!qaEnabled}>
              <MessageSquareText className="mr-2 h-4 w-4" />
              {t('detail.tabs.qa')}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="preview" className="mt-4">
            <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(360px,0.9fr)]">
              <FileOriginalPreviewPanel
                file={file}
                onDownload={canDownload ? () => void handleDownload() : undefined}
                isDownloading={isDownloading}
              />
              <FileParseReviewPanel fileId={file.id} enabled={parseReviewEnabled} />
            </div>
          </TabsContent>

          <TabsContent value="chunks" className="mt-4">
            <FileChunksPanel fileId={file.id} enabled={chunksEnabled} />
          </TabsContent>

          <TabsContent value="qa" className="mt-4">
            <FileIndexInfoPanel
              artifactState={artifactState}
              processing={processing}
              vectorStatus={vectorStatus}
              enabled={qaEnabled}
            />
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
