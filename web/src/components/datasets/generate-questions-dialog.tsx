import React, { useState } from 'react';
import { useT } from '@/i18n';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { ModelSelector } from '@/components/common/model-selector';
import { Loader2 } from 'lucide-react';
import type { SegmentDetail } from '@/services/types/dataset';

interface GenerateQuestionsDialogProps {
  open: boolean;
  onClose: () => void;
  segment?: SegmentDetail;
  isLoading?: boolean;
  onGenerate: (segmentId: string, model: { provider: string; name: string }) => Promise<void>;
}

export function GenerateQuestionsDialog({
  open,
  onClose,
  segment,
  isLoading,
  onGenerate,
}: GenerateQuestionsDialogProps) {
  const t = useT();
  const [modelConfig, setModelConfig] = useState<{
    provider: string;
    model: string;
    mode: string;
  }>({
    provider: 'openai',
    model: 'gpt-3.5-turbo',
    mode: 'chat',
  });

  const handleConfirm = async () => {
    if (!segment || !modelConfig.provider || !modelConfig.model) return;

    await onGenerate(segment.id, {
      provider: modelConfig.provider,
      name: modelConfig.model,
    });
    onClose();
  };

  return (
    <Dialog open={open} onOpenChange={onClose}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>{t('datasets.generateQuestions')}</DialogTitle>
        </DialogHeader>
        <DialogBody className="grid gap-4 py-4">
          <div className="space-y-2">
            <label className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70">
              {t('common.model')}
            </label>
            <ModelSelector
              value={{
                provider: modelConfig.provider,
                model: modelConfig.model,
              }}
              onChange={value => {
                setModelConfig({
                  provider: value.provider,
                  model: value.model,
                  mode: 'chat',
                });
              }}
              modelType="text-chat"
            />
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={isLoading}>
            {t('common.cancel')}
          </Button>
          <Button onClick={handleConfirm} disabled={isLoading || !modelConfig.model}>
            {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            {t('common.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
