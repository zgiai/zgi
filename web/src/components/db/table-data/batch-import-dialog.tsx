'use client';

// Batch import dialog component for DB table data
// English comments for maintainability and clarity

import type { FC } from 'react';
import React, { useCallback, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Download, Loader, X, FileSpreadsheet } from 'lucide-react';
import {
  useDownloadDbTableTemplate,
  useImportDbTableRecords,
} from '@/hooks/db/use-db-table-import';
import { toast } from 'sonner';
import { getErrorMessage } from '@/utils/error-notifications';
import { useT } from '@/i18n';
import FileSelectorDialog from '@/components/files/file-selector-dialog';
import type { FileItem } from '@/services/types/file';

export interface BatchImportDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  dbId: string;
  tableId: string;
  onSuccess?: () => void;
}

const BatchImportDialog: FC<BatchImportDialogProps> = ({
  open,
  onOpenChange,
  dbId,
  tableId,
  onSuccess,
}) => {
  const t = useT();
  const [selectedFile, setSelectedFile] = useState<FileItem | null>(null);
  const [fileSelectorOpen, setFileSelectorOpen] = useState(false);

  // Template download hook
  const { downloadTemplate, isDownloading } = useDownloadDbTableTemplate(dbId, tableId);

  // Import records hook
  const {
    importRecords,
    isPending: isImporting,
    reset: resetImport,
  } = useImportDbTableRecords(dbId, tableId);

  // Handle template download
  const handleDownloadTemplate = async () => {
    try {
      await downloadTemplate();
      toast.success(t('dbs.batchImport.downloadSuccess'));
    } catch (err) {
      toast.error(getErrorMessage(err) || t('dbs.batchImport.downloadFailed'));
    }
  };

  // Remove selected file
  const handleRemoveFile = () => {
    setSelectedFile(null);
  };

  const handleFileConfirm = useCallback((files: FileItem[]) => {
    const file = files[0];
    if (!file) return;
    const ext = file.extension?.toLowerCase();
    if (ext !== 'xlsx' && ext !== 'xls') {
      toast.error(t('dbs.batchImport.invalidFileType'));
      return;
    }
    setSelectedFile(file);
  }, [t]);

  // Handle import action
  const handleImport = async () => {
    if (!selectedFile) return;
    try {
      await importRecords(selectedFile);
      // Toast is handled in the hook
      // Reset state and close dialog
      setSelectedFile(null);
      resetImport();
      onOpenChange(false);
      onSuccess?.();
    } catch {
      // Error toast is handled in the hook
    }
  };

  // Handle dialog close
  const handleOpenChange = useCallback(
    (newOpen: boolean) => {
      if (!newOpen) {
        // Reset state when closing
        setSelectedFile(null);
        resetImport();
      }
      onOpenChange(newOpen);
    },
    [onOpenChange, resetImport]
  );

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle>{t('dbs.batchImport.title')}</DialogTitle>
        </DialogHeader>

        <DialogBody className="space-y-6">
          {/* Step 1: Download Template */}
          <div className="space-y-3">
            <h3 className="text-base font-medium">{t('dbs.batchImport.step1Title')}</h3>
            <p className="text-sm text-muted-foreground">{t('dbs.batchImport.step1Desc')}</p>
            <Button onClick={handleDownloadTemplate} disabled={isDownloading} className="gap-2">
              {isDownloading ? (
                <Loader className="h-4 w-4 animate-spin" />
              ) : (
                <Download className="h-4 w-4" />
              )}
              {t('dbs.batchImport.downloadTemplate')}
            </Button>
          </div>

          {/* Step 2: Upload File */}
          <div className="space-y-3">
            <h3 className="text-base font-medium">{t('dbs.batchImport.step2Title')}</h3>

            {selectedFile ? (
              // File selected - show file info
              <div className="flex items-center justify-between p-3 border rounded-md bg-muted/50">
                <div className="flex items-center gap-2">
                  <FileSpreadsheet className="h-5 w-5 text-green-600" />
                  <span className="text-sm font-medium truncate max-w-[280px]">
                    {selectedFile.name}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    ({(selectedFile.size / 1024).toFixed(1)} KB)
                  </span>
                </div>
                <Button
                  variant="ghost"
                  isIcon
                  className="h-6 w-6"
                  onClick={handleRemoveFile}
                  disabled={isImporting}
                >
                  <X className="h-4 w-4" />
                </Button>
              </div>
            ) : (
              <div className="flex flex-col items-center justify-center rounded-md border p-8 text-center">
                <FileSpreadsheet className="h-8 w-8 text-muted-foreground mb-3" />
                <p className="text-sm text-foreground mb-1">{t('dbs.batchImport.dropOrClick')}</p>
                <p className="text-xs text-muted-foreground">
                  {t('dbs.batchImport.supportedFormats')}
                </p>
                <Button
                  variant="outline"
                  size="sm"
                  className="mt-4"
                  onClick={() => setFileSelectorOpen(true)}
                >
                  {t('dbs.batchImport.selectFile')}
                </Button>
              </div>
            )}
          </div>
        </DialogBody>

        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)} disabled={isImporting}>
            {t('dbs.batchImport.cancel')}
          </Button>
          <Button onClick={handleImport} disabled={!selectedFile || isImporting}>
            {isImporting ? (
              <>
                <Loader className="h-4 w-4 animate-spin mr-2" />
                {t('dbs.batchImport.importing')}
              </>
            ) : (
              t('dbs.batchImport.import')
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
      <FileSelectorDialog
        open={fileSelectorOpen}
        onOpenChange={setFileSelectorOpen}
        onConfirm={handleFileConfirm}
        initSelectedFiles={selectedFile ? [selectedFile] : []}
        maxCount={1}
        acceptExt={['xlsx', 'xls']}
      />
    </Dialog>
  );
};

export default BatchImportDialog;
