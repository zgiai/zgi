'use client';

import { useT } from '@/i18n';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { PromptPlaygroundPanel } from './prompt-playground-panel';
import type { PromptPlaygroundMessage } from '@/services/types/prompt';
import type { ModelSelectorValue } from '@/components/common/model-selector';

interface PromptPlaygroundDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  prefillPromptId?: string;
  prefillPromptText?: string;
  prefillPromptMessages?: PromptPlaygroundMessage[];
  prefillPromptLabel?: string;
  prefillModel?: ModelSelectorValue | null;
}

export function PromptPlaygroundDialog({
  open,
  onOpenChange,
  prefillPromptId,
  prefillPromptText,
  prefillPromptMessages,
  prefillPromptLabel,
  prefillModel,
}: PromptPlaygroundDialogProps) {
  const t = useT('prompts');

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-6xl">
        <DialogHeader>
          <DialogTitle>{t('playground.title')}</DialogTitle>
        </DialogHeader>
        <DialogBody>
          <PromptPlaygroundPanel
            prefillPromptId={prefillPromptId}
            prefillPromptText={prefillPromptText}
            prefillPromptMessages={prefillPromptMessages}
            prefillPromptLabel={prefillPromptLabel}
            prefillModel={prefillModel}
          />
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

export default PromptPlaygroundDialog;
