'use client';

import { useEffect, useMemo, useState } from 'react';
import { AlertTriangle } from 'lucide-react';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { useUpdateChannelUpstreamSettings } from '@/hooks';
import { useT } from '@/i18n';
import type { ChannelItem, UpstreamWarningThreshold } from '@/services/types/channel';

interface ChannelUpstreamSettingsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  channel: ChannelItem | null;
}

const DECIMAL_PATTERN = /^\d+(?:\.\d+)?$/;

export function ChannelUpstreamSettingsDialog({
  open,
  onOpenChange,
  channel,
}: ChannelUpstreamSettingsDialogProps): JSX.Element {
  const t = useT('channels');
  const state = channel?.upstream_state;
  const currencies = useMemo(() => {
    const values = new Set<string>();
    state?.balances?.forEach(item => values.add(item.currency));
    state?.warning_thresholds?.forEach(item => values.add(item.currency));
    return Array.from(values).sort();
  }, [state]);
  const [enabled, setEnabled] = useState<Record<string, boolean>>({});
  const [amounts, setAmounts] = useState<Record<string, string>>({});
  const { updateUpstreamSettings, isUpdating } = useUpdateChannelUpstreamSettings();

  useEffect(() => {
    if (!open) return;
    const nextEnabled: Record<string, boolean> = {};
    const nextAmounts: Record<string, string> = {};
    const thresholds = new Map(
      (state?.warning_thresholds ?? []).map(item => [item.currency, item.amount])
    );
    currencies.forEach(currency => {
      const amount = thresholds.get(currency);
      nextEnabled[currency] = amount !== undefined;
      nextAmounts[currency] = amount ?? '';
    });
    setEnabled(nextEnabled);
    setAmounts(nextAmounts);
  }, [currencies, open, state?.warning_thresholds]);

  const invalid = currencies.some(
    currency => enabled[currency] && !DECIMAL_PATTERN.test((amounts[currency] ?? '').trim())
  );
  const supportsThresholds = state?.balance_capability === 'supported';
  const hasConfiguredThresholds = (state?.warning_thresholds?.length ?? 0) > 0;
  const hasEnabledThresholds = currencies.some(currency => enabled[currency]);
  const canConfigure = currencies.length > 0 && (supportsThresholds || hasConfiguredThresholds);
  const canSubmit =
    canConfigure && !invalid && (supportsThresholds || !hasEnabledThresholds) && !isUpdating;

  const submit = async () => {
    if (!channel || invalid) return;
    const warningThresholds: UpstreamWarningThreshold[] = currencies
      .filter(currency => enabled[currency])
      .map(currency => ({ currency, amount: amounts[currency].trim() }));
    await updateUpstreamSettings(channel.id, { warning_thresholds: warningThresholds });
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[calc(100vw-24px)] max-w-md">
        <DialogHeader>
          <DialogTitle>{t('upstream.settingsTitle')}</DialogTitle>
          <DialogDescription>{channel?.name}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          {(state?.shared_channel_count ?? 0) > 1 && (
            <div className="flex gap-2 rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              <span>
                {t('upstream.sharedCredential', { count: state?.shared_channel_count ?? 0 })}
              </span>
            </div>
          )}

          {canConfigure ? (
            <div className="space-y-3">
              {currencies.map(currency => (
                <div key={currency} className="grid grid-cols-[auto_52px_1fr] items-center gap-2">
                  <Checkbox
                    checked={Boolean(enabled[currency])}
                    disabled={!supportsThresholds && !enabled[currency]}
                    onCheckedChange={checked =>
                      setEnabled(current => ({ ...current, [currency]: checked === true }))
                    }
                    aria-label={t('upstream.enableThreshold', { currency })}
                  />
                  <span className="text-sm font-medium">{currency}</span>
                  <Input
                    inputMode="decimal"
                    value={amounts[currency] ?? ''}
                    onChange={event =>
                      setAmounts(current => ({ ...current, [currency]: event.target.value }))
                    }
                    placeholder={t('upstream.thresholdPlaceholder')}
                    disabled={!enabled[currency] || !supportsThresholds}
                    aria-invalid={enabled[currency] && !DECIMAL_PATTERN.test(amounts[currency] ?? '')}
                  />
                </div>
              ))}
              <p className="text-xs text-muted-foreground">
                {supportsThresholds
                  ? t('upstream.pollingHint')
                  : t('upstream.clearOnlyHint')}
              </p>
            </div>
          ) : (
            <div className="rounded-md border bg-muted/30 p-3 text-sm text-muted-foreground">
              {t('upstream.thresholdUnavailable')}
            </div>
          )}

          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              {t('actions.cancel')}
            </Button>
            <Button onClick={submit} disabled={!canSubmit}>
              {t('actions.confirm')}
            </Button>
          </div>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
