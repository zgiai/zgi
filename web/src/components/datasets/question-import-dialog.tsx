'use client';

import React, { useRef, useState } from 'react';
import { useT } from '@/i18n';
import { Upload, FileText } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
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

interface QuestionImportDialogProps {
  open: boolean;
  onClose: () => void;
  onImport: (file: File) => Promise<void> | void;
}

export function QuestionImportDialog({ open, onClose, onImport }: QuestionImportDialogProps) {
  const t = useT('datasets');
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [dragOver, setDragOver] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [isImporting, setIsImporting] = useState(false);

  const handleTemplateDownload = () => {
    const csvContent = `question\n${t('questionImport.exampleQuestion1')}\n${t('questionImport.exampleQuestion2')}\n`;
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'questions_template.csv';
    a.click();
    URL.revokeObjectURL(url);
  };

  const validateFile = (f: File) => {
    if (!f.name.toLowerCase().endsWith('.csv')) {
      toast.error(t('questionImport.selectCsvFile'));
      return false;
    }
    if (f.size > 10 * 1024 * 1024) {
      toast.error(t('questionImport.fileSizeTooLarge'));
      return false;
    }
    return true;
  };

  const handleSelect = (files: FileList | null) => {
    if (!files || files.length === 0) return;
    const f = files[0];
    if (!validateFile(f)) return;
    setFile(f);
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    handleSelect(e.dataTransfer.files);
  };

  const handleImport = async () => {
    if (!file) {
      toast.error(t('questionImport.selectFileFirst'));
      return;
    }
    setIsImporting(true);
    try {
      await onImport(file);
      toast(t('questionImport.importCompleted'));
      setFile(null);
      onClose();
    } catch (_e) {
      toast.error(t('questionImport.importFailed'));
    } finally {
      setIsImporting(false);
    }
  };

  const handleClose = () => {
    if (isImporting) return;
    setFile(null);
    onClose();
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('questionImport.title')}</DialogTitle>
          <DialogDescription>{t('questionImport.description')}</DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-6">
          <div className="space-y-2">
            <Label>{t('questionImport.csvStructure')}</Label>
            <div className="text-xs text-muted-foreground">{t('questionImport.columnLabel')}</div>
            <Button
              variant="link"
              size="sm"
              className="px-0 text-blue-600 text-sm"
              onClick={handleTemplateDownload}
            >
              {t('questionImport.downloadTemplate')}
            </Button>
          </div>

          <div
            className={`border-2 border-dashed rounded-lg p-6 text-center transition-colors ${
              dragOver ? 'border-primary bg-primary/5' : 'border-muted-foreground/25'
            }`}
            onDragOver={e => {
              e.preventDefault();
              setDragOver(true);
            }}
            onDragLeave={e => {
              e.preventDefault();
              setDragOver(false);
            }}
            onDrop={handleDrop}
            onClick={() => fileInputRef.current?.click()}
          >
            <input
              ref={fileInputRef}
              type="file"
              accept=".csv"
              className="hidden"
              onChange={e => handleSelect(e.target.files)}
            />
            {file ? (
              <div className="space-y-2">
                <FileText className="h-8 w-8 mx-auto text-primary" />
                <div>
                  <p className="font-medium">{file.name}</p>
                  <p className="text-sm text-muted-foreground">
                    {(file.size / 1024).toFixed(1)} KB
                  </p>
                </div>
                <Button variant="outline" size="sm" onClick={() => setFile(null)}>
                  {t('questionImport.reselect')}
                </Button>
              </div>
            ) : (
              <div className="space-y-2">
                <Upload className="h-8 w-8 mx-auto text-muted-foreground" />
                <div>
                  <p className="font-medium">{t('questionImport.dragOrClick')}</p>
                  <p className="text-sm text-muted-foreground">
                    {t('questionImport.supportedFormat')}
                  </p>
                </div>
              </div>
            )}
          </div>
        </DialogBody>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={isImporting}>
            {t('questionImport.cancel')}
          </Button>
          <Button onClick={handleImport} disabled={!file || isImporting}>
            {isImporting ? t('questionImport.importing') : t('questionImport.import')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
