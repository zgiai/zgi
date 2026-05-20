'use client';

// Table prompt management dialog component
// English comments only; strict types; reuse TanStack Query hooks for fetch/update & toasts

import React, { useEffect, useMemo, useState } from 'react';
import { AlertCircle } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { useDbTablePrompt, useUpdateDbTablePrompt } from '@/hooks/db/use-db-table-prompt';
import { useT } from '@/i18n';

interface TablePromptDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  dbId: string;
  tableId: string;
}

export function TablePromptDialog(props: TablePromptDialogProps) {
  const { open, onOpenChange, dbId, tableId } = props;
  const { prompt, isLoading } = useDbTablePrompt(dbId, tableId, { enabled: open });
  const { updatePrompt, isPending } = useUpdateDbTablePrompt(dbId, tableId);
  const t = useT();

  // Default prompt content used for "Reset to default" action
  const FALLBACK_DEFAULT_PROMPT = '请基于数据内容和业务场景，合理推断并创建合适的表结构。';
  const defaultPromptText = t('dbs.promptDialog.defaultText');
  // Local editable value synced from server prompt
  const initialValue = useMemo(() => prompt ?? '', [prompt]);
  const [value, setValue] = useState<string>(initialValue);

  useEffect(() => {
    // Sync textarea when dialog opens or prompt updates
    if (open) setValue(initialValue);
  }, [open, initialValue]);

  const handleSave = async () => {
    await updatePrompt(value);
    onOpenChange(false);
  };

  const handleResetDefault = () => {
    setValue((defaultPromptText as string) || FALLBACK_DEFAULT_PROMPT);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('dbs.promptDialog.title')}</DialogTitle>
        </DialogHeader>

        <DialogBody className="space-y-4">
          {/* Label row with reset link */}
          <div className="flex items-center justify-between">
            <Label className="text-sm">{t('dbs.promptDialog.contentLabel')}</Label>
            <button
              type="button"
              onClick={handleResetDefault}
              className="text-sm text-muted-foreground hover:text-foreground"
            >
              {t('dbs.promptDialog.resetDefault')}
            </button>
          </div>

          {/* Textarea (skeleton on first load) */}
          {isLoading ? (
            <Skeleton className="h-40 w-full" />
          ) : (
            <Textarea
              value={value}
              onChange={e => setValue(e.target.value)}
              placeholder={(defaultPromptText as string) || FALLBACK_DEFAULT_PROMPT}
              className="min-h-[160px] resize-none"
            />
          )}

          {/* Hint area */}
          <div className="flex items-center gap-2 rounded-md border border-highlight px-3 py-2 text-sm text-highlight bg-highlight/10">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <div>{t('dbs.promptDialog.hintText')}</div>
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isPending}>
            {t('common.cancel')}
          </Button>
          <Button onClick={handleSave} disabled={isPending}>
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default TablePromptDialog;
