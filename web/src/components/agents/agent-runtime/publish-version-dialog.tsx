'use client';

import { useEffect, useState } from 'react';
import { UploadCloud } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { useT } from '@/i18n';
import type { AgentPublishVersionDetails } from './types';

const VERSION_NAME_MAX_LENGTH = 80;
const VERSION_DESCRIPTION_MAX_LENGTH = 500;

interface AgentPublishVersionDialogProps {
  open: boolean;
  isUpdate: boolean;
  isSubmitting: boolean;
  value: AgentPublishVersionDetails;
  onOpenChange: (open: boolean) => void;
  onChange: (value: AgentPublishVersionDetails) => void;
  onConfirm: () => void;
}

function formatFallbackTime(timestamp: number): string {
  return new Date(timestamp).toLocaleString(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  });
}

export function AgentPublishVersionDialog({
  open,
  isUpdate,
  isSubmitting,
  value,
  onOpenChange,
  onChange,
  onConfirm,
}: AgentPublishVersionDialogProps) {
  const t = useT('agents.agentRuntime.publishVersion');
  const [openedAt, setOpenedAt] = useState(() => Date.now());

  useEffect(() => {
    if (open) setOpenedAt(Date.now());
  }, [open]);

  const fallbackName = t('fallbackName', { time: formatFallbackTime(openedAt) });
  const versionNameLength = Array.from(value.name).length;

  return (
    <Dialog
      open={open}
      onOpenChange={nextOpen => {
        if (!isSubmitting) onOpenChange(nextOpen);
      }}
    >
      <DialogContent size="md" className="p-0 text-left" showCloseButton={!isSubmitting}>
        <form
          className="flex min-h-0 flex-1 flex-col"
          onSubmit={event => {
            event.preventDefault();
            if (!isSubmitting) onConfirm();
          }}
        >
          <DialogHeader>
            <div className="flex items-start gap-3">
              <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <UploadCloud className="size-5" />
              </div>
              <div className="min-w-0">
                <DialogTitle>{t(isUpdate ? 'updateTitle' : 'publishTitle')}</DialogTitle>
                <DialogDescription className="mt-1.5">
                  {t(isUpdate ? 'updateDescription' : 'publishDescription')}
                </DialogDescription>
              </div>
            </div>
          </DialogHeader>

          <DialogBody className="space-y-5">
            <div className="space-y-2">
              <div className="flex items-center justify-between gap-3">
                <label htmlFor="agent-version-name" className="text-sm font-medium">
                  {t('nameLabel')}
                  <span className="ml-1 font-normal text-muted-foreground">{t('optional')}</span>
                </label>
                <span className="text-xs text-muted-foreground">
                  {versionNameLength}/{VERSION_NAME_MAX_LENGTH}
                </span>
              </div>
              <Input
                id="agent-version-name"
                value={value.name}
                maxLength={VERSION_NAME_MAX_LENGTH}
                placeholder={t('namePlaceholder')}
                autoFocus
                disabled={isSubmitting}
                onChange={event =>
                  onChange({
                    ...value,
                    name: event.target.value.slice(0, VERSION_NAME_MAX_LENGTH),
                  })
                }
              />
              <p className="text-xs leading-5 text-muted-foreground">
                {value.name.trim() ? t('nameHelp') : t('fallbackPreview', { name: fallbackName })}
              </p>
            </div>

            <div className="space-y-2">
              <label htmlFor="agent-version-description" className="text-sm font-medium">
                {t('descriptionLabel')}
                <span className="ml-1 font-normal text-muted-foreground">{t('optional')}</span>
              </label>
              <Textarea
                id="agent-version-description"
                value={value.description}
                maxLength={VERSION_DESCRIPTION_MAX_LENGTH}
                showCharacterCount
                className="min-h-28 resize-none"
                placeholder={t('descriptionPlaceholder')}
                disabled={isSubmitting}
                onChange={event =>
                  onChange({
                    ...value,
                    description: event.target.value.slice(0, VERSION_DESCRIPTION_MAX_LENGTH),
                  })
                }
              />
              <p className="text-xs leading-5 text-muted-foreground">{t('descriptionHelp')}</p>
            </div>
          </DialogBody>

          <DialogFooter className="border-t bg-muted/20">
            <Button
              type="button"
              variant="ghost"
              disabled={isSubmitting}
              onClick={() => onOpenChange(false)}
            >
              {t('cancel')}
            </Button>
            <Button type="submit" loading={isSubmitting} disabled={isSubmitting}>
              {t(isSubmitting ? (isUpdate ? 'updating' : 'publishing') : isUpdate ? 'update' : 'publish')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
