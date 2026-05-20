'use client';

import React, { useState, useMemo, useEffect } from 'react';
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
import { Skeleton } from '@/components/ui/skeleton';
import { Loader2, TrendingUp, TrendingDown, Minus } from 'lucide-react';
import { useT } from '@/i18n';
import { useAdjustChannelWallet, useChannel } from '@/hooks';
import { cn } from '@/lib/utils';
import {
  DEFAULT_AI_CREDIT_EDIT_MAX,
  formatChannelCreditPoints,
  formatChannelCreditUsd,
  sanitizeAiCreditIntegerInput,
} from '@/utils/ai-credits';

export interface ChannelWalletAdjustDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  channel: { id: string; name: string } | null;
}

/**
 * Dialog for adjusting private channel wallet balance.
 * User inputs target balance (0 ~ 99,999,999), system calculates delta.
 * Fetches latest channel detail (including balance) when dialog opens.
 */
export default function ChannelWalletAdjustDialog({
  open,
  onOpenChange,
  channel,
}: ChannelWalletAdjustDialogProps): JSX.Element {
  const t = useT('channels');
  const { adjustWallet, isAdjusting } = useAdjustChannelWallet();
  const maxBalanceLabel = DEFAULT_AI_CREDIT_EDIT_MAX.toLocaleString();

  // Fetch channel detail to get latest balance when dialog opens
  const { channel: channelDetail, isLoading: isLoadingChannel } = useChannel(
    open ? channel?.id : undefined
  );

  // Current balance from fetched channel detail (parse string to number)
  const currentBalance = useMemo(() => {
    if (!channelDetail?.balance) return 0;
    const parsed = parseFloat(channelDetail.balance);
    return Number.isFinite(parsed) ? parsed : 0;
  }, [channelDetail?.balance]);

  // Form state
  const [targetBalanceStr, setTargetBalanceStr] = useState('');
  const [note, setNote] = useState('');
  const [initialized, setInitialized] = useState(false);

  // Reset form when dialog opens or channel detail loads
  useEffect(() => {
    if (open && !isLoadingChannel && channelDetail && !initialized) {
      setTargetBalanceStr(currentBalance.toString());
      setNote('');
      setInitialized(true);
    }
    if (!open) {
      setInitialized(false);
    }
  }, [open, isLoadingChannel, channelDetail, currentBalance, initialized]);

  // Parse target balance
  const targetBalance = useMemo(() => {
    const parsed = parseFloat(targetBalanceStr);
    return Number.isFinite(parsed) ? parsed : 0;
  }, [targetBalanceStr]);

  // Calculate adjustment delta
  const adjustmentDelta = useMemo(() => {
    return targetBalance - currentBalance;
  }, [targetBalance, currentBalance]);

  // Validation
  const validation = useMemo(() => {
    if (targetBalanceStr.trim() === '') {
      return { valid: false, error: t('walletAdjust.validation.required') };
    }
    if (targetBalance < 0) {
      return { valid: false, error: t('walletAdjust.validation.min') };
    }
    if (targetBalance > DEFAULT_AI_CREDIT_EDIT_MAX) {
      return { valid: false, error: t('walletAdjust.validation.max', { max: maxBalanceLabel }) };
    }
    if (adjustmentDelta === 0) {
      return { valid: false, error: t('walletAdjust.validation.noChange') };
    }
    return { valid: true, error: null };
  }, [adjustmentDelta, maxBalanceLabel, t, targetBalance, targetBalanceStr]);

  // Handle input change - only allow non-negative integers
  const handleTargetChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    setTargetBalanceStr(sanitizeAiCreditIntegerInput(value, DEFAULT_AI_CREDIT_EDIT_MAX));
  };

  // Handle submit
  const handleSubmit = async () => {
    if (!channel || !validation.valid || adjustmentDelta === 0) return;

    await adjustWallet(channel.id, {
      amount: adjustmentDelta,
      note: note.trim() || undefined,
    });

    onOpenChange(false);
  };

  // Render adjustment indicator
  const renderAdjustmentIndicator = () => {
    if (adjustmentDelta === 0) {
      return (
        <div className="flex items-center gap-2 text-muted-foreground">
          <Minus className="h-4 w-4" />
          <span>{t('walletAdjust.noChange')}</span>
        </div>
      );
    }

    if (adjustmentDelta > 0) {
      return (
        <div className="flex items-center gap-2 text-green-600">
          <TrendingUp className="h-4 w-4" />
          <span>
            +{formatChannelCreditPoints(adjustmentDelta)} {t('credit.points')} ·{' '}
            {formatChannelCreditUsd(adjustmentDelta)} ({t('walletAdjust.increase')})
          </span>
        </div>
      );
    }

    return (
      <div className="flex items-center gap-2 text-red-600">
        <TrendingDown className="h-4 w-4" />
        <span>
          -{formatChannelCreditPoints(Math.abs(adjustmentDelta))} {t('credit.points')} ·{' '}
          {formatChannelCreditUsd(Math.abs(adjustmentDelta))} ({t('walletAdjust.decrease')})
        </span>
      </div>
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t('walletAdjust.title')}</DialogTitle>
          <DialogDescription>
            {t('walletAdjust.description')}
            {channel?.name && (
              <span className="font-medium text-foreground ml-1">({channel.name})</span>
            )}
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-4">
          {isLoadingChannel ? (
            // Loading skeleton while fetching channel detail
            <div className="space-y-4">
              <div className="space-y-2">
                <Skeleton className="h-4 w-20" />
                <Skeleton className="h-10 w-full" />
              </div>
              <div className="space-y-2">
                <Skeleton className="h-4 w-20" />
                <Skeleton className="h-10 w-full" />
              </div>
              <div className="space-y-2">
                <Skeleton className="h-4 w-20" />
                <Skeleton className="h-10 w-full" />
              </div>
            </div>
          ) : (
            <>
              {/* Current Balance (read-only) */}
              <div className="space-y-2">
                <label className="text-sm font-medium">{t('walletAdjust.currentBalance')}</label>
                <div className="rounded-md border bg-muted/30 px-3 py-2">
                  <div className="text-sm font-semibold">
                    {formatChannelCreditPoints(currentBalance)} {t('credit.points')}
                  </div>
                  <div className="text-xs text-muted-foreground">
                    {t('credit.approxUsd', { amount: formatChannelCreditUsd(currentBalance) })}
                  </div>
                </div>
              </div>

              {/* Target Balance Input */}
              <div className="space-y-2">
                <label className="text-sm font-medium">{t('walletAdjust.targetBalance')}</label>
                <div className="flex items-center gap-2">
                  <Input
                    type="text"
                    inputMode="numeric"
                    value={targetBalanceStr}
                    onChange={handleTargetChange}
                    placeholder="0"
                    className={cn(
                      'font-mono',
                      validation.error && targetBalanceStr !== '' && 'border-destructive'
                    )}
                  />
                  <div className="flex h-10 shrink-0 items-center rounded-md border bg-background px-3 text-sm text-muted-foreground">
                    {t('credit.points')}
                  </div>
                </div>
                <p className="text-xs text-muted-foreground">
                  {t('credit.approxUsd', { amount: formatChannelCreditUsd(targetBalance) })} ·{' '}
                  {t('walletAdjust.rateHint')}
                </p>
                <p className="text-xs text-muted-foreground">
                  {t('walletAdjust.maxLimit', { max: maxBalanceLabel })}
                </p>
              </div>

              {/* Adjustment Preview */}
              <div className="space-y-2">
                <label className="text-sm font-medium">{t('walletAdjust.adjustAmount')}</label>
                <div className="px-3 py-2 bg-muted/50 rounded-md text-sm font-mono">
                  {renderAdjustmentIndicator()}
                </div>
              </div>

              {/* Note (optional) */}
              <div className="space-y-2">
                <label className="text-sm font-medium">{t('walletAdjust.note')}</label>
                <Input
                  value={note}
                  onChange={e => setNote(e.target.value)}
                  placeholder={t('walletAdjust.notePlaceholder')}
                />
              </div>

              {/* Validation Error */}
              {validation.error && targetBalanceStr !== '' && (
                <p className="text-sm text-destructive">{validation.error}</p>
              )}
            </>
          )}
        </DialogBody>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isAdjusting}>
            {t('walletAdjust.buttons.cancel')}
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={isAdjusting || isLoadingChannel || !validation.valid}
          >
            {isAdjusting && <Loader2 className="h-4 w-4 mr-1 animate-spin" />}
            {t('walletAdjust.buttons.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
