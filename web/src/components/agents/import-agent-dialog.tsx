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
import { Label } from '@/components/ui/label';
import {
  WorkspaceSelector,
  type WorkspaceSelectorValue,
} from '@/components/common/workspace-selector';
import { FileUpload, type FileUploadRef } from '@/components/common/file-upload';
import { useImportWorkflow } from '@/hooks/workflow/use-workflow-import-export';
import { useIsOrganizationMode } from '@/store/workspace-store';

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
  const [selectedWorkspace, setSelectedWorkspace] = useState<WorkspaceSelectorValue | undefined>();
  const isOrganizationMode = useIsOrganizationMode();
  const effectiveWorkspaceId = isOrganizationMode ? selectedWorkspace?.id : workspaceId;
  const { importWorkflow, isImporting } = useImportWorkflow();

  const handleCancel = useCallback(() => {
    fileUploadRef.current?.clearAll();
    setSelectedCount(0);
    setSelectedWorkspace(undefined);
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
    setSelectedWorkspace(undefined);
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
          {isOrganizationMode ? (
            <div className="space-y-2.5">
              <Label className="text-sm font-semibold">{t('agents.form.workspace')}</Label>
              <WorkspaceSelector
                value={selectedWorkspace}
                placeholder={t('agents.form.workspacePlaceholder')}
                autoSelectFirst
                onChange={setSelectedWorkspace}
              />
            </div>
          ) : null}
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
            disabled={
              selectedCount === 0 || isImporting || (isOrganizationMode && !effectiveWorkspaceId)
            }
          >
            {isImporting ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : null}
            {isImporting ? t('agents.importingAgent') : t('agents.importAgent')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
