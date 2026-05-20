'use client';

import React, { useState, useRef } from 'react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Progress } from '@/components/ui/progress';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { toast } from 'sonner';
import { Upload, Download, FileText, CheckCircle, XCircle } from 'lucide-react';

import type { ProcessStatus } from '@/services/types/dataset';

interface BatchImportDialogProps {
  open: boolean;
  onClose: () => void;
  isLoading: boolean;
  progress?: {
    status: ProcessStatus;
    message?: string;
    percentage?: number;
  };
  onImport: (file: File) => Promise<void>;
  onDownloadTemplate: () => void;
}

export function BatchImportDialog({
  open,
  onClose,
  isLoading,
  progress,
  onImport,
  onDownloadTemplate,
}: BatchImportDialogProps) {
  const t = useT('datasets');
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [dragOver, setDragOver] = useState(false);
  const [selectedFile, setSelectedFile] = useState<File | null>(null);

  // Handle file selection
  const handleFileSelect = (files: FileList | null) => {
    if (!files || files.length === 0) return;

    const file = files[0];
    if (!file.name.toLowerCase().endsWith('.csv')) {
      toast.error(t('batchImport.selectCsvFile'));
      return;
    }

    if (file.size > 10 * 1024 * 1024) {
      // 10MB limit
      toast.error(t('batchImport.fileSizeLimit'));
      return;
    }

    setSelectedFile(file);
  };

  // Handle drag and drop
  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(true);
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    handleFileSelect(e.dataTransfer.files);
  };

  // Handle file input click
  const handleFileInputClick = () => {
    fileInputRef.current?.click();
  };

  // Handle import
  const handleImport = async () => {
    if (!selectedFile) {
      toast.error(t('batchImport.selectFileFirst'));
      return;
    }

    try {
      await onImport(selectedFile);
    } catch (_error) {
      toast.error(t('batchImport.importFailed'));
    }
  };

  // Handle dialog close
  const handleClose = () => {
    if (!isLoading) {
      setSelectedFile(null);
      onClose();
    }
  };

  // Get progress status
  const getProgressStatus = () => {
    if (!progress) return null;

    switch (progress.status) {
      case 'waiting':
        return {
          icon: <FileText className="h-4 w-4" />,
          text: t('batchImport.statusWaiting'),
          color: 'text-blue-500',
        };
      case 'processing':
        return {
          icon: <Upload className="h-4 w-4" />,
          text: t('batchImport.statusProcessing'),
          color: 'text-yellow-500',
        };
      case 'completed':
        return {
          icon: <CheckCircle className="h-4 w-4" />,
          text: t('batchImport.statusCompleted'),
          color: 'text-green-500',
        };
      case 'error':
        return {
          icon: <XCircle className="h-4 w-4" />,
          text: t('batchImport.statusError'),
          color: 'text-red-500',
        };
      default:
        return null;
    }
  };

  const progressStatus = getProgressStatus();

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('batchImport.title')}</DialogTitle>
          <DialogDescription>{t('batchImport.description')}</DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-6">
          {/* Template download */}
          <div className="space-y-2">
            <Label>{t('batchImport.templateDownload')}</Label>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={onDownloadTemplate}
                className="flex items-center gap-2"
              >
                <Download className="h-4 w-4" />
                {t('batchImport.downloadTemplate')}
              </Button>
              <span className="text-xs text-muted-foreground">{t('batchImport.templateHint')}</span>
            </div>
          </div>

          {/* File upload */}
          <div className="space-y-2">
            <Label>{t('batchImport.selectFile')}</Label>
            <div
              className={`border-2 border-dashed rounded-lg p-6 text-center transition-colors ${
                dragOver
                  ? 'border-primary bg-primary/5'
                  : 'border-muted-foreground/25 hover:border-muted-foreground/50'
              }`}
              onDragOver={handleDragOver}
              onDragLeave={handleDragLeave}
              onDrop={handleDrop}
              onClick={handleFileInputClick}
            >
              <input
                ref={fileInputRef}
                type="file"
                accept=".csv"
                onChange={e => handleFileSelect(e.target.files)}
                className="hidden"
              />

              {selectedFile ? (
                <div className="space-y-2">
                  <FileText className="h-8 w-8 mx-auto text-primary" />
                  <div>
                    <p className="font-medium">{selectedFile.name}</p>
                    <p className="text-sm text-muted-foreground">
                      {(selectedFile.size / 1024).toFixed(1)} KB
                    </p>
                  </div>
                  <Button variant="outline" size="sm" onClick={() => setSelectedFile(null)}>
                    {t('batchImport.reselect')}
                  </Button>
                </div>
              ) : (
                <div className="space-y-2">
                  <Upload className="h-8 w-8 mx-auto text-muted-foreground" />
                  <div>
                    <p className="font-medium">{t('batchImport.dragOrClick')}</p>
                    <p className="text-sm text-muted-foreground">
                      {t('batchImport.fileFormatHint')}
                    </p>
                  </div>
                </div>
              )}
            </div>
          </div>

          {/* Progress */}
          {progress && (
            <div className="space-y-3">
              <div className="flex items-center gap-2">
                {progressStatus && (
                  <>
                    <span className={progressStatus.color}>{progressStatus.icon}</span>
                    <span className="font-medium">{progressStatus.text}</span>
                  </>
                )}
              </div>

              {progress.percentage !== undefined && (
                <div className="space-y-1">
                  <Progress value={progress.percentage} className="h-2" />
                  <div className="flex justify-between text-xs text-muted-foreground">
                    <span>{t('batchImport.progress')}</span>
                    <span>{progress.percentage}%</span>
                  </div>
                </div>
              )}

              {progress.message && (
                <p className="text-sm text-muted-foreground">{progress.message}</p>
              )}
            </div>
          )}

          {/* Format instructions */}
          <div className="p-4 bg-muted rounded-lg text-sm space-y-2">
            <h4 className="font-medium">{t('batchImport.formatTitle')}</h4>
            <ul className="space-y-1 text-muted-foreground">
              <li>• {t('batchImport.formatRequired')}</li>
              <li>• {t('batchImport.formatOptional')}</li>
              <li>• {t('batchImport.formatEncoding')}</li>
              <li>• {t('batchImport.formatOnePerLine')}</li>
            </ul>
          </div>
        </DialogBody>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={isLoading}>
            {progress?.status === 'completed' ? t('batchImport.close') : t('actions.cancel')}
          </Button>
          {progress?.status !== 'completed' && (
            <Button onClick={handleImport} disabled={!selectedFile || isLoading}>
              {isLoading ? t('batchImport.importing') : t('batchImport.startImport')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
