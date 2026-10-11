'use client';

import { useEffect, useState } from 'react';
import { AlertTriangle, CheckCircle2, UploadCloud } from 'lucide-react';
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
import { cn } from '@/lib/utils';

export const DIGITAL_HUMAN_VERSION_NAME_MAX_LENGTH = 80;
export const DIGITAL_HUMAN_VERSION_DESCRIPTION_MAX_LENGTH = 500;

export interface DigitalHumanPublishVersionDetails {
  name: string;
  description: string;
}

interface PublishCheck {
  label: string;
  ready: boolean;
  blocking: boolean;
}

interface DigitalHumanPublishVersionDialogProps {
  open: boolean;
  isUpdate: boolean;
  isSubmitting: boolean;
  hasUnpublishedChanges: boolean;
  isLoadingBindings: boolean;
  checks: PublishCheck[];
  value: DigitalHumanPublishVersionDetails;
  onOpenChange: (open: boolean) => void;
  onChange: (value: DigitalHumanPublishVersionDetails) => void;
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

export function DigitalHumanPublishVersionDialog({
  open,
  isUpdate,
  isSubmitting,
  hasUnpublishedChanges,
  isLoadingBindings,
  checks,
  value,
  onOpenChange,
  onChange,
  onConfirm,
}: DigitalHumanPublishVersionDialogProps) {
  const t = useT('digitalHumans');
  const [openedAt, setOpenedAt] = useState(() => Date.now());
  const blockingIssues = checks.filter(check => check.blocking && !check.ready);
  const warningCount = checks.filter(check => !check.blocking && !check.ready).length;

  useEffect(() => {
    if (open) setOpenedAt(Date.now());
  }, [open]);

  const fallbackName = t('publishDialog.fallbackName', {
    time: formatFallbackTime(openedAt),
  });

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
                <DialogTitle>
                  {t(isUpdate ? 'publishDialog.updateTitle' : 'publishDialog.publishTitle')}
                </DialogTitle>
                <DialogDescription className="mt-1.5">
                  {t(
                    isUpdate
                      ? 'publishDialog.updateDescription'
                      : 'publishDialog.publishDescription'
                  )}
                </DialogDescription>
              </div>
            </div>
          </DialogHeader>

          <DialogBody className="space-y-5">
            <div className="space-y-2">
              <label htmlFor="digital-human-version-name" className="text-sm font-medium">
                {t('publishDialog.nameLabel')}
                <span className="ml-1 font-normal text-muted-foreground">
                  {t('publishDialog.optional')}
                </span>
              </label>
              <Input
                id="digital-human-version-name"
                value={value.name}
                maxLength={DIGITAL_HUMAN_VERSION_NAME_MAX_LENGTH}
                showCharacterCount
                placeholder={t('publishDialog.namePlaceholder')}
                autoFocus
                disabled={isSubmitting}
                onChange={event => onChange({ ...value, name: event.target.value })}
              />
              <p className="text-xs leading-5 text-muted-foreground">
                {value.name.trim()
                  ? t('publishDialog.nameHelp')
                  : t('publishDialog.fallbackPreview', { name: fallbackName })}
              </p>
            </div>

            <div className="space-y-2">
              <label htmlFor="digital-human-version-description" className="text-sm font-medium">
                {t('publishDialog.descriptionLabel')}
                <span className="ml-1 font-normal text-muted-foreground">
                  {t('publishDialog.optional')}
                </span>
              </label>
              <Textarea
                id="digital-human-version-description"
                value={value.description}
                maxLength={DIGITAL_HUMAN_VERSION_DESCRIPTION_MAX_LENGTH}
                showCharacterCount
                className="min-h-24 resize-none"
                placeholder={t('publishDialog.descriptionPlaceholder')}
                disabled={isSubmitting}
                onChange={event => onChange({ ...value, description: event.target.value })}
              />
              <p className="text-xs leading-5 text-muted-foreground">
                {t('publishDialog.descriptionHelp')}
              </p>
            </div>

            <div className="rounded-lg border bg-muted/20 p-3">
              <div className="mb-2 text-sm font-medium">{t('configuration.preflight.title')}</div>
              <div className="space-y-2">
                {checks.map(check => (
                  <div key={check.label} className="flex items-center gap-2 text-sm">
                    {check.ready ? (
                      <CheckCircle2 className="size-4 shrink-0 text-emerald-600" />
                    ) : (
                      <AlertTriangle
                        className={cn(
                          'size-4 shrink-0',
                          check.blocking ? 'text-destructive' : 'text-amber-500'
                        )}
                      />
                    )}
                    <span className={check.ready ? 'text-foreground' : 'text-muted-foreground'}>
                      {check.label}
                    </span>
                  </div>
                ))}
              </div>
            </div>
            {warningCount > 0 && blockingIssues.length === 0 ? (
              <p className="text-xs leading-5 text-amber-600 dark:text-amber-400">
                {t('configuration.preflight.warning', { count: warningCount })}
              </p>
            ) : null}
            <p className="text-xs leading-5 text-muted-foreground">
              {t('publishDialog.immutableHint')}
            </p>
          </DialogBody>

          <DialogFooter className="border-t bg-muted/20">
            <Button
              type="button"
              variant="ghost"
              disabled={isSubmitting}
              onClick={() => onOpenChange(false)}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="submit"
              loading={isSubmitting}
              disabled={
                !hasUnpublishedChanges ||
                isSubmitting ||
                isLoadingBindings ||
                blockingIssues.length > 0
              }
            >
              {t(isUpdate ? 'publishDialog.update' : 'publishDialog.publish')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
