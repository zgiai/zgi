'use client';

import React from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { useUpdateOfficialChannelSettings } from '@/hooks';
import type { PlatformChannelsResponse } from '@/services/types/channel';
import { useT } from '@/i18n';

export interface OfficialChannelDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initial?: PlatformChannelsResponse | null;
}

/** Utility: number or undefined */
function toNumberOrUndefined(value: string): number | undefined {
  const v = value.trim();
  if (!v) return undefined;
  const n = Number(v);
  return Number.isFinite(n) ? n : undefined;
}

/** Utility: sanitize numeric input to allow only non-negative integers */
function sanitizeNonNegativeInt(value: string): string {
  // Remove all non-digit characters
  const digits = value.replace(/\D/g, '');
  // Remove leading zeros but keep single '0'
  return digits.replace(/^0+(?=\d)/, '');
}

function OfficialChannelForm({
  initial,
  onOpenChange,
}: {
  initial: PlatformChannelsResponse;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useT('channels');

  const [priority, setPriority] = React.useState<string>(
    initial.priority != null ? String(initial.priority) : ''
  );
  const [weight, setWeight] = React.useState<string>(
    initial.weight != null ? String(initial.weight) : ''
  );

  const { updateOfficialSettings, isUpdating } = useUpdateOfficialChannelSettings();

  const onSubmit = async () => {
    await updateOfficialSettings({
      priority: toNumberOrUndefined(priority),
      weight: toNumberOrUndefined(weight),
      is_enabled: initial.is_enabled,
    });
    onOpenChange(false);
  };

  return (
    <div className="flex flex-col gap-6">
      <DialogHeader>
        <DialogTitle>{t('dialog.titleEdit')}</DialogTitle>
        <DialogDescription>{t('description')}</DialogDescription>
      </DialogHeader>

      <DialogBody className="space-y-6">
        {/* Read-only Info */}
        <div className="bg-muted/30 rounded-lg p-4 space-y-3 border">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium text-muted-foreground">
              {t('dialog.labels.name')}
            </span>
            <span className="text-sm font-semibold">{initial.name}</span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium text-muted-foreground">{t('table.models')}</span>
            <span className="text-sm">{initial.model_count}</span>
          </div>
        </div>

        {/* Editable Fields */}
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <label className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70">
              {t('dialog.labels.priority')}
            </label>
            <Input
              inputMode="numeric"
              value={priority}
              onChange={e => setPriority(sanitizeNonNegativeInt(e.target.value))}
              placeholder={t('dialog.placeholders.priority')}
              min={0}
            />
          </div>
          <div className="space-y-2">
            <label className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70">
              {t('dialog.labels.weight')}
            </label>
            <Input
              inputMode="numeric"
              value={weight}
              onChange={e => setWeight(sanitizeNonNegativeInt(e.target.value))}
              placeholder={t('dialog.placeholders.weight')}
              min={0}
            />
          </div>
        </div>
      </DialogBody>

      <DialogFooter className="flex justify-end gap-3 pt-4 border-t">
        <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isUpdating}>
          {t('dialog.buttons.cancel')}
        </Button>
        <Button onClick={onSubmit} disabled={isUpdating}>
          {t('dialog.buttons.save')}
        </Button>
      </DialogFooter>
    </div>
  );
}

export default function OfficialChannelDialog({
  open,
  onOpenChange,
  initial,
}: OfficialChannelDialogProps): JSX.Element {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        {open && initial && <OfficialChannelForm initial={initial} onOpenChange={onOpenChange} />}
      </DialogContent>
    </Dialog>
  );
}
