'use client';

import React, { useState, useEffect, useRef } from 'react';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Slider } from '@/components/ui/slider';
import { useT } from '@/i18n';
import { FileTypeSelector } from '../../common/file-type-selector';
import { FileExtensionEditor } from '../../common/file-extension-editor';
import type { WorkflowFeatures, FileUploadType } from '../../store/type';

interface FileUploadSettingsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  value: WorkflowFeatures['file_upload'] | undefined;
  onChange: (value: Partial<WorkflowFeatures['file_upload']> | undefined) => void;
}

const FileUploadSettingsDialog: React.FC<FileUploadSettingsDialogProps> = ({
  open,
  onOpenChange,
  value,
  onChange,
}) => {
  const t = useT('agents');
  const tCommon = useT('common');

  const [localTypes, setLocalTypes] = useState<FileUploadType[]>(value?.allowed_file_types ?? []);
  const [exts, setExts] = useState<string[]>(value?.allowed_file_extensions ?? []);
  const [limit, setLimit] = useState<number>(value?.number_limits ?? 3);

  const syncingFromPropsRef = useRef(false);

  useEffect(() => {
    if (open) {
      syncingFromPropsRef.current = true;
      setLocalTypes(value?.allowed_file_types ?? []);
      // Normalize extensions to have dot prefix
      setExts(
        (value?.allowed_file_extensions ?? []).map(e => {
          const cleaned = e.toLowerCase().replace(/^\./, '');
          return `.${cleaned}`;
        })
      );
      const nl = value && typeof value.number_limits === 'number' ? value.number_limits : 3;
      setLimit(nl);
      // Allow next effect to run without triggering onChange immediately
      window.setTimeout(() => {
        syncingFromPropsRef.current = false;
      }, 0);
    }
  }, [open, value]);

  const isCustom = localTypes.includes('custom');

  useEffect(() => {
    if (!open || syncingFromPropsRef.current) return;
    const next: Partial<WorkflowFeatures['file_upload']> = {
      allowed_file_types: localTypes,
      // Store extensions without dot prefix for API compatibility
      allowed_file_extensions: exts.map(e => e.toLowerCase().replace(/^\./, '')),
      number_limits: Math.max(1, Math.min(10, limit)),
    };
    onChange(next);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, localTypes, exts, limit]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[600px] p-0 overflow-hidden">
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-bold tracking-tight">
            {t('workflow.features.uploadLabel')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="space-y-8 py-6">
          <div className="space-y-6">
            <FileTypeSelector
              selectedTypes={localTypes}
              onChange={types => setLocalTypes(types as FileUploadType[])}
              label={t('workflow.features.allowedTypes')}
            />

            {isCustom && (
              <div className="pt-2">
                <FileExtensionEditor
                  extensions={exts}
                  onChange={setExts}
                  placeholder={t('workflow.features.customPlaceholder')}
                  label={t('workflow.features.allowedExtensions')}
                  clearText={tCommon('clear')}
                />
              </div>
            )}

            <div className="space-y-4 pt-4 border-t border-neutral-100">
              <div className="flex items-center justify-between">
                <Label className="text-sm font-bold tracking-tight">
                  {t('workflow.features.totalFileCount')}
                </Label>
                <span className="text-sm font-bold bg-primary/10 text-primary px-3 py-1 rounded-full">
                  {limit}
                </span>
              </div>
              <div className="px-1">
                <Slider
                  value={[limit]}
                  min={1}
                  max={10}
                  step={1}
                  onValueChange={(vals: number[]) => setLimit(Math.max(1, vals[0] ?? 1))}
                />
              </div>
              <p className="text-xs text-muted-foreground font-medium px-1">
                {t('workflow.features.fileCountSuffix', { count: limit })}
              </p>
            </div>
          </div>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
          <Button
            type="button"
            variant="ghost"
            onClick={() => onOpenChange(false)}
            className="font-semibold"
          >
            {tCommon('close')}
          </Button>
          <Button
            type="button"
            onClick={() => onOpenChange(false)}
            size="lg"
            className="px-8 font-bold"
          >
            {tCommon('save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default FileUploadSettingsDialog;
