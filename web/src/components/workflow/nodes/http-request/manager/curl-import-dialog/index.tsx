'use client';

import React, { useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { Textarea } from '@/components/ui/textarea';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { useConvertCurl } from '@/hooks/use-convert-curl';
import type { ConvertCurlResult } from '@/utils/curl';
import { useT } from '@/i18n';

interface CurlImportDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // Called when cURL has been successfully converted
  onImportSuccess: (data: ConvertCurlResult) => void;
}

const CurlImportDialog: React.FC<CurlImportDialogProps> = ({
  open,
  onOpenChange,
  onImportSuccess,
}) => {
  const t = useT();
  const [curlInput, setCurlInput] = useState<string>('');

  const { mutate: convertCurl, isPending } = useConvertCurl({
    onSuccess: data => {
      // Notify parent to apply data; toast is handled inside the hook
      onImportSuccess(data);
      // Reset local state and close dialog
      setCurlInput('');
      onOpenChange(false);
    },
  });

  const handleKeyDownCapture = React.useCallback((e: React.KeyboardEvent) => {
    // Prevent key events inside the dialog from affecting the canvas/editor
    e.stopPropagation();
  }, []);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-[600px] p-0 overflow-hidden"
        onKeyDownCapture={handleKeyDownCapture}
      >
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-bold tracking-tight">
            {t('nodes.httpRequest.actions.importFromCurl')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="space-y-6 py-6 font-medium">
          <div className="space-y-4">
            <Textarea
              id="curl-input"
              placeholder={t('nodes.httpRequest.placeholders.curlExample')}
              className="min-h-[200px] max-h-[400px] shadow-sm font-medium resize-none bg-neutral-50/50 border-neutral-200 focus-visible:ring-primary/20"
              rows={10}
              value={curlInput}
              onChange={e => setCurlInput(e.target.value)}
            />
            {isPending && (
              <div className="space-y-3 animate-in fade-in">
                <Skeleton className="h-4 w-1/3 rounded-full" />
                <Skeleton className="h-4 w-2/3 rounded-full" />
                <Skeleton className="h-24 w-full rounded-2xl" />
              </div>
            )}
          </div>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
          <Button variant="ghost" onClick={() => onOpenChange(false)} className="font-semibold">
            {t('nodes.httpRequest.actions.cancel')}
          </Button>
          <Button
            onClick={() => convertCurl({ curl: curlInput })}
            size="lg"
            className="px-10 font-bold shadow-sm"
            disabled={!curlInput.trim() || isPending}
          >
            {t('nodes.httpRequest.actions.import')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default CurlImportDialog;
