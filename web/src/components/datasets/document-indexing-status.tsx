'use client';

import { useEffect } from 'react';
import { useDocumentIndexingInfo } from '@/hooks/dataset/use-document-detail';
import { Progress } from '@/components/ui/progress';
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { formatDate } from '@/utils/format';
import { DocumentIndexingStatus } from '@/services/types/dataset';
import { LoaderIcon } from 'lucide-react';
import { useT } from '@/i18n';

interface DocumentIndexingStatusViewProps {
  datasetId: string;
  documentId: string;
}

export function DocumentIndexingStatusView({
  datasetId,
  documentId,
}: DocumentIndexingStatusViewProps) {
  // Fetch indexing progress info
  const { progressData, isLoading, isError, refetchStatus } = useDocumentIndexingInfo(
    datasetId,
    documentId
  );
  const t = useT();

  // Compute completion percentage — prefer API progress field, fallback to segment ratio
  const percent =
    typeof progressData?.progress === 'number'
      ? progressData.progress
      : progressData && progressData.total_segments > 0
        ? Math.floor((progressData.completed_segments / progressData.total_segments) * 100)
        : 0;

  const TERMINAL = new Set(['completed', 'error', 'paused']);

  // Polling for index status every 1 second until terminal status
  useEffect(() => {
    if (TERMINAL.has(String(progressData?.indexing_status || ''))) {
      return; // Do not start polling in terminal states
    }
    const timer = setInterval(() => {
      refetchStatus && refetchStatus();
    }, 1000);
    return () => clearInterval(timer);
  }, [refetchStatus, progressData?.indexing_status]);

  // Loading skeleton for first request
  if (isLoading) {
    return <Skeleton className="h-64 w-full" />;
  }

  // Error state or missing data
  if (isError || !progressData) {
    return <div>{t('datasets.indexingStatus.loadFailed')}</div>;
  }

  // Helper to render timestamp safely
  const renderTime = (ts: number | null | undefined) => {
    return typeof ts === 'number' && isFinite(ts) ? formatDate(ts) : '-';
  };

  const statusTextMap: Record<DocumentIndexingStatus, string> = {
    [DocumentIndexingStatus.WAITING]: t('datasets.indexingStatus.statusText.waiting'),
    [DocumentIndexingStatus.PARSING]: t('datasets.indexingStatus.statusText.parsing'),
    [DocumentIndexingStatus.CLEANING]: t('datasets.indexingStatus.statusText.cleaning'),
    [DocumentIndexingStatus.SPLITTING]: t('datasets.indexingStatus.statusText.splitting'),
    [DocumentIndexingStatus.INDEXING]: t('datasets.indexingStatus.statusText.indexing'),
    [DocumentIndexingStatus.PAUSED]: t('datasets.indexingStatus.statusText.paused'),
    [DocumentIndexingStatus.ERROR]: t('datasets.indexingStatus.statusText.error'),
    [DocumentIndexingStatus.COMPLETED]: t('datasets.indexingStatus.statusText.completed'),
    [DocumentIndexingStatus.EXTRACTING]: t('datasets.indexingStatus.statusText.extracting'),
    [DocumentIndexingStatus.ALIGNMENT]: t('datasets.indexingStatus.statusText.alignment'),
    [DocumentIndexingStatus.INGESTING]: t('datasets.indexingStatus.statusText.ingesting'),
  };

  return (
    <>
      {/* Indexing progress information */}
      <Card>
        <CardHeader>
          <CardTitle>{t('datasets.indexingStatus.progressTitle')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
            <p>
              <strong>{t('datasets.indexingStatus.status')}:</strong>{' '}
              {statusTextMap[progressData.indexing_status]}
            </p>
            <p>
              <strong>{t('datasets.indexingStatus.progress')}:</strong> {percent}%
            </p>
          </div>
          <div className="grid grid-cols-1 gap-2">
            <p className="flex items-center gap-2">
              <strong>{t('datasets.indexingStatus.completedSegments')}:</strong>{' '}
              {progressData.completed_segments}/{progressData.total_segments}
              {!TERMINAL.has(String(progressData.indexing_status)) && (
                <LoaderIcon className="w-4 h-4 animate-spin" />
              )}
            </p>
            <Progress value={percent} className="w-full" />
          </div>
          {progressData.graph_indexing_status && (
            <div className="grid grid-cols-1 gap-2">
              <p className="flex items-center gap-2">
                <strong>{t('datasets.indexingStatus.graphStatus')}:</strong>{' '}
                {statusTextMap[progressData.graph_indexing_status as DocumentIndexingStatus] ||
                  progressData.graph_indexing_status}
                {!TERMINAL.has(progressData.graph_indexing_status) && (
                  <LoaderIcon className="w-4 h-4 animate-spin" />
                )}
              </p>
            </div>
          )}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 pt-2">
            <p>
              <strong>{t('datasets.indexingStatus.startTime')}:</strong>{' '}
              {renderTime(progressData.processing_started_at)}
            </p>
            {progressData.parsing_completed_at && (
              <p>
                <strong>{t('datasets.indexingStatus.parsingCompleted')}:</strong>{' '}
                {renderTime(progressData.parsing_completed_at)}
              </p>
            )}
            {progressData.cleaning_completed_at && (
              <p>
                <strong>{t('datasets.indexingStatus.cleaningCompleted')}:</strong>{' '}
                {renderTime(progressData.cleaning_completed_at)}
              </p>
            )}
            {progressData.splitting_completed_at && (
              <p>
                <strong>{t('datasets.indexingStatus.splittingCompleted')}:</strong>{' '}
                {renderTime(progressData.splitting_completed_at)}
              </p>
            )}
            {progressData.completed_at && (
              <p>
                <strong>{t('datasets.indexingStatus.completedTime')}:</strong>{' '}
                {renderTime(progressData.completed_at)}
              </p>
            )}
            {progressData.paused_at && (
              <p>
                <strong>{t('datasets.indexingStatus.pausedTime')}:</strong>{' '}
                {renderTime(progressData.paused_at)}
              </p>
            )}
            {progressData.stopped_at && (
              <p>
                <strong>{t('datasets.indexingStatus.stoppedTime')}:</strong>{' '}
                {renderTime(progressData.stopped_at)}
              </p>
            )}
          </div>
          {progressData.error && (
            <div className="text-sm text-destructive bg-destructive/10 rounded-md p-2">
              <strong>{t('datasets.indexingStatus.error')}:</strong> {progressData.error}
            </div>
          )}
        </CardContent>
      </Card>
    </>
  );
}
