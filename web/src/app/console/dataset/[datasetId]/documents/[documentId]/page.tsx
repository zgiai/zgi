'use client';

import { useEffect } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { ArrowLeft, ExternalLink } from 'lucide-react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { useDocumentDetail } from '@/hooks/dataset/use-document-detail';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import {
  PermissionDeniedState,
  PermissionLoadingState,
} from '@/components/common/permission-gate-state';
import { KNOWLEDGE_BASE_PERMISSION_ACTIONS } from '@/constants/permissions';

export default function DocumentDetailRedirectPage() {
  const params = useParams();
  const router = useRouter();
  const t = useT();
  const datasetId = params.datasetId as string;
  const documentId = params.documentId as string;
  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canViewDocuments = hasAnyPermission([
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentView,
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentCreate,
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentUpdate,
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentDelete,
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.indexManage,
  ]);

  const { document, metadata, isLoading, documentError } = useDocumentDetail({
    datasetId,
    documentId,
    enabled: Boolean(datasetId && documentId && canViewDocuments),
  });

  const sourceFileId =
    (typeof metadata?.doc_metadata?.source_file_id === 'string'
      ? metadata.doc_metadata.source_file_id
      : undefined) || document?.data_source_info?.upload_file?.id;

  useEffect(() => {
    if (sourceFileId) {
      const returnTo = `/console/dataset/${datasetId}/documents`;
      router.replace(`/console/files/${sourceFileId}?returnTo=${encodeURIComponent(returnTo)}`);
    }
  }, [datasetId, router, sourceFileId]);

  if (isPermissionsLoading) {
    return <PermissionLoadingState />;
  }

  if (!canViewDocuments) {
    return <PermissionDeniedState />;
  }

  if (isLoading && !documentError) {
    return (
      <div className="space-y-4 p-6">
        <Skeleton className="h-8 w-56" />
        <Skeleton className="h-24 w-full" />
      </div>
    );
  }

  return (
    <div className="flex min-h-full items-center justify-center p-8 text-center">
      <div className="max-w-md space-y-4">
        <ExternalLink className="mx-auto h-10 w-10 text-muted-foreground" />
        <div>
          <h1 className="text-lg font-semibold">
            {t('datasets.documents.fileRefs.redirectTitle')}
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {t('datasets.documents.fileRefs.redirectDescription')}
          </p>
        </div>
        <Button
          variant="outline"
          onClick={() => router.push(`/console/dataset/${datasetId}/documents`)}
        >
          <ArrowLeft className="h-4 w-4" />
          {t('datasets.backToDocuments')}
        </Button>
      </div>
    </div>
  );
}
