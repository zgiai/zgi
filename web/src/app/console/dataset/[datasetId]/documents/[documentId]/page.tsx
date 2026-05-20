'use client';

import { useParams, useRouter } from 'next/navigation';
import { useT } from '@/i18n';
import { ArrowLeft, AlertCircle, ShieldAlert } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';

import { useDocumentDetail } from '@/hooks/dataset/use-document-detail';
import { useDataset } from '@/hooks/dataset/use-datasets';
import { DocumentIndexingStatusView } from '@/components/datasets/document-indexing-status';
import { SegmentList } from '@/components/datasets/segment-list';
import { ExtractionFallbackNotice } from '@/components/datasets/document/extraction-fallback-notice';

export default function DocumentDetailPage() {
  const params = useParams();
  const router = useRouter();
  const t = useT();

  const datasetId = params.datasetId as string;
  const documentId = params.documentId as string;

  // Get dataset to check edit permission
  const { data: datasetData, isLoading: isDatasetLoading } = useDataset(datasetId);
  const canEdit = datasetData?.data?.can_edit ?? false;

  // Only fetch document detail when user has edit permission
  const shouldFetchDocument = !isDatasetLoading && canEdit;
  const { document, metadata, isLoading, documentError } = useDocumentDetail({
    datasetId,
    documentId,
    enabled: shouldFetchDocument,
  });

  const handleBack = () => {
    router.push(`/console/dataset/${datasetId}/documents`);
  };

  // Loading state - wait for dataset first, then document if user has permission
  if (isDatasetLoading || (shouldFetchDocument && isLoading)) {
    return (
      <div className="space-y-6 p-6">
        <div className="flex items-center gap-4">
          <Skeleton className="h-10 w-10 rounded" />
          <Skeleton className="h-8 w-48" />
        </div>
        <Skeleton className="h-96" />
      </div>
    );
  }

  // Permission denied state - show empty state when user lacks edit permission
  if (!canEdit) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-4 text-center p-8">
        <div className="w-16 h-16 rounded-full bg-muted flex items-center justify-center">
          <ShieldAlert className="w-8 h-8 text-muted-foreground" />
        </div>
        <div className="space-y-2">
          <h2 className="text-lg font-semibold text-foreground">
            {t('datasets.noEditPermission')}
          </h2>
          <p className="text-sm text-muted-foreground max-w-md">
            {t('datasets.noEditPermissionDescription')}
          </p>
        </div>
        <Button onClick={handleBack} variant="outline">
          <ArrowLeft className="h-4 w-4 mr-2" />
          {t('datasets.backToDocuments')}
        </Button>
      </div>
    );
  }

  // Error state
  if (documentError) {
    return (
      <div className="text-center py-12">
        <AlertCircle className="h-12 w-12 text-destructive mx-auto mb-4" />
        <h3 className="text-lg font-medium mb-2">{t('datasets.documentLoadFailed')}</h3>
        <p className="text-muted-foreground mb-4">
          {documentError instanceof Error ? documentError.message : t('datasets.unknownError')}
        </p>
        <Button onClick={handleBack} variant="outline">
          <ArrowLeft className="h-4 w-4 mr-2" />
          {t('datasets.backToDocuments')}
        </Button>
      </div>
    );
  }

  // Document not found
  if (!document) {
    return (
      <div className="text-center py-12">
        <AlertCircle className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
        <h3 className="text-lg font-medium mb-2">{t('datasets.documentNotFound')}</h3>
        <Button onClick={handleBack} variant="outline">
          <ArrowLeft className="h-4 w-4 mr-2" />
          {t('datasets.backToDocuments')}
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-6 p-[24px]">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center gap-4 justify-between">
        <div className="flex items-center gap-2 overflow-hidden">
          <Button variant="ghost" isIcon className="h-8 w-8 shrink-0" onClick={handleBack}>
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <h1 className="text-xl font-semibold truncate" title={document.name}>
            {document.name}
          </h1>
        </div>
      </div>

      <ExtractionFallbackNotice extraction={metadata?.doc_metadata?.extraction} variant="banner" />

      {!(document.indexing_status === 'completed') ? (
        <DocumentIndexingStatusView datasetId={datasetId} documentId={documentId} />
      ) : (
        <SegmentList datasetId={datasetId} documentId={documentId} />
      )}
    </div>
  );
}
