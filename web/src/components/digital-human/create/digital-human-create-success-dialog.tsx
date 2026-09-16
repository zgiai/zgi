'use client';

import type { IconValue } from '@/components/common/icon-input/types';
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
import { useT } from '@/i18n';
import {
  DigitalHumanAvatar,
  DigitalHumanCapabilityList,
  DigitalHumanTagList,
} from './digital-human-template-preview';

/**
 * Created-and-published dialog.
 *
 * Creation replaces the draft bindings once with only the items actually bound —
 * the skills the user selected and the workspace really offers — and then
 * publishes version 1, so the dialog reports a published digital human and lists
 * the capabilities and resources that were really bound. Every destination is
 * reachable for a published digital human.
 */
export function DigitalHumanCreateSuccessDialog({
  open,
  onOpenChange,
  name,
  templateTitle,
  responsibility,
  tags,
  boundCapabilityNames,
  icon,
  onContinueSetup,
  onChannels,
  onAutonomy,
  onBackToList,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  name: string;
  templateTitle: string;
  responsibility: string;
  tags: readonly string[];
  /** Display names of the items that were actually bound, in bind order. */
  boundCapabilityNames: readonly string[];
  icon: IconValue;
  onContinueSetup: () => void;
  onChannels: () => void;
  onAutonomy: () => void;
  onBackToList: () => void;
}) {
  const t = useT('digitalHumans');
  const summary = responsibility.trim();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg" className="max-h-[calc(100vh-2rem)]">
        <DialogHeader className="items-center text-center">
          <DigitalHumanAvatar icon={icon} fallbackName={name} size="lg" />
          <DialogTitle className="mt-2">{t('createFlow.success.title', { name })}</DialogTitle>
          <DialogDescription className="leading-6">
            {t('createFlow.success.intro', {
              name,
              role: templateTitle,
              responsibility: summary,
            })}
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-5">
          <p className="rounded-lg bg-muted/50 p-3 text-xs leading-5 text-muted-foreground">
            {t('createFlow.success.written')}
          </p>

          {boundCapabilityNames.length > 0 ? (
            <div className="space-y-2 rounded-lg border border-success/30 bg-success/5 p-3">
              <p className="text-xs font-medium text-success">
                {t('createFlow.success.boundCapabilities')}
              </p>
              <DigitalHumanCapabilityList items={boundCapabilityNames} />
            </div>
          ) : null}

          <div className="grid gap-3 sm:grid-cols-2">
            <div className="rounded-lg border p-3">
              <p className="text-xs font-medium text-success">{t('createFlow.success.completedTitle')}</p>
              <p className="mt-1.5 text-xs leading-5 text-muted-foreground">
                {t('createFlow.success.completedItems')}
              </p>
            </div>
            <div className="rounded-lg border p-3">
              <p className="text-xs font-medium text-muted-foreground">
                {t('createFlow.success.pendingTitle')}
              </p>
              <p className="mt-1.5 text-xs leading-5 text-muted-foreground">
                {t('createFlow.success.pendingItems')}
              </p>
            </div>
          </div>

          {tags.length > 0 ? (
            <div className="space-y-2">
              <p className="text-xs font-medium text-muted-foreground">
                {t('createFlow.success.strengths')}
              </p>
              <DigitalHumanTagList tags={tags} />
            </div>
          ) : null}

          <div className="space-y-3">
            <p className="text-sm font-semibold">{t('createFlow.success.nextTitle')}</p>
            <div className="grid gap-3 sm:grid-cols-2">
              <button
                type="button"
                onClick={onChannels}
                className="rounded-xl border p-4 text-left outline-none transition-colors motion-reduce:transition-none hover:border-border hover:bg-muted/30 focus-visible:ring-2 focus-visible:ring-ring"
              >
                <p className="text-sm font-medium">{t('createFlow.success.imTitle')}</p>
                <p className="mt-1.5 text-xs leading-5 text-muted-foreground">
                  {t('createFlow.success.imDescription')}
                </p>
                <p className="mt-1.5 text-[11px] leading-5 text-muted-foreground">
                  {t('createFlow.success.imNote')}
                </p>
              </button>
              <button
                type="button"
                onClick={onAutonomy}
                className="rounded-xl border p-4 text-left outline-none transition-colors motion-reduce:transition-none hover:border-border hover:bg-muted/30 focus-visible:ring-2 focus-visible:ring-ring"
              >
                <p className="text-sm font-medium">{t('createFlow.success.autonomyTitle')}</p>
                <p className="mt-1.5 text-xs leading-5 text-muted-foreground">
                  {t('createFlow.success.autonomyDescription')}
                </p>
              </button>
            </div>
          </div>
        </DialogBody>

        <DialogFooter className="flex-col gap-2 sm:flex-row sm:justify-end">
          <Button variant="outline" className="w-full sm:w-auto" onClick={onBackToList}>
            {t('createFlow.success.backToList')}
          </Button>
          <Button className="w-full sm:w-auto" onClick={onContinueSetup}>
            {t('createFlow.success.continueSetup')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
