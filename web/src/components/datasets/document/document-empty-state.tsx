import React from 'react';
import { Database, Plus, ShieldAlert } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';

interface DocumentEmptyStateProps {
  /** Optional callback - button hidden when undefined (e.g., user lacks edit permission) */
  onCreateDocument?: () => void;
  isExternalDataSource?: boolean;
  /** Whether user has edit permission - shows different message when false */
  canEdit?: boolean;
}

export function DocumentEmptyState({
  onCreateDocument,
  isExternalDataSource,
  canEdit = true,
}: DocumentEmptyStateProps) {
  const t = useT('datasets');

  // Show add button only when callback provided and not external data source
  const showAddButton = !isExternalDataSource && !!onCreateDocument;

  // Show different content when user lacks edit permission
  if (!canEdit) {
    return (
      <div className="border rounded-md py-12 text-center">
        <div className="mx-auto w-24 h-24 bg-muted rounded-full flex items-center justify-center mb-4">
          <ShieldAlert className="h-8 w-8 text-muted-foreground" />
        </div>
        <h3 className="text-lg font-medium mb-2">{t('noDocuments')}</h3>
        <p className="text-muted-foreground mb-4 max-w-md mx-auto">
          {t('documents.noEditPermissionHint')}
        </p>
      </div>
    );
  }

  return (
    <div className="border rounded-md py-12 text-center">
      <div className="mx-auto w-24 h-24 bg-muted rounded-full flex items-center justify-center mb-4">
        <Database className="h-8 w-8 text-muted-foreground" />
      </div>

      <h3 className="text-lg font-medium mb-2">{t('noDocuments')}</h3>

      <p className="text-muted-foreground mb-4">{t('documents.uploadFirstDocument')}</p>

      {showAddButton && (
        <Button onClick={onCreateDocument}>
          <Plus className="h-4 w-4 mr-2" />
          {t('documents.addFirstDocument')}
        </Button>
      )}
    </div>
  );
}
