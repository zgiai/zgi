'use client';

import { useCallback, useRef, useState } from 'react';
import { RefreshCw } from 'lucide-react';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { FileUpload, type FileUploadRef } from '@/components/common/file-upload';
import { useImportWorkflow } from '@/hooks/workflow/use-workflow-import-export';

interface ImportAgentDialogProps {
  open: boolean;
  workspaceId?: string;
  onOpenChange: (open: boolean) => void;
  onImportComplete?: () => void | Promise<void>;
}

export default function ImportAgentDialog({
  open,
  workspaceId,
  onOpenChange,
  onImportComplete,
}: ImportAgentDialogProps) {
  const t = useT();
  const fileUploadRef = useRef<FileUploadRef>(null);
  const [selectedCount, setSelectedCount] = useState(0);
  const effectiveWorkspaceId = workspaceId;
  const { importWorkflow, isImporting } = useImportWorkflow();

  const handleCancel = useCallback(() => {
    fileUploadRef.current?.clearAll();
    setSelectedCount(0);
    onOpenChange(false);
  }, [onOpenChange]);

  const handleImport = useCallback(async () => {
    const pendingFiles = fileUploadRef.current?.getPendingFiles() ?? [];
    const file = pendingFiles[0];
    if (!file) {
      toast.error(t('agents.importSelectFile'));
      return;
    }
    if (!effectiveWorkspaceId) {
      toast.error(t('agents.validation.workspace.required'));
      return;
    }
    await importWorkflow({ file, workspaceId: effectiveWorkspaceId });
    fileUploadRef.current?.clearAll();
    setSelectedCount(0);
    onOpenChange(false);
    await onImportComplete?.();
  }, [effectiveWorkspaceId, importWorkflow, onImportComplete, onOpenChange, t]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[600px] max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t('agents.importAgent')}</DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-4 py-4">
          <FileUpload
            ref={fileUploadRef}
            autoUpload={false}
            maxCount={1}
            maxSizeMB={10}
            acceptExt={['yml', 'yaml']}
            onFilesChange={files => setSelectedCount(files.length)}
          />
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={handleCancel} disabled={isImporting}>
            {t('common.cancel')}
          </Button>
          <Button
            onClick={handleImport}
            disabled={selectedCount === 0 || isImporting || !effectiveWorkspaceId}
          >
            {isImporting ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : null}
            {isImporting ? t('agents.importingAgent') : t('agents.importAgent')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
