'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import type { FormEvent } from 'react';
import { Save } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { PricingFallbackPanel } from '@/components/settings/pricing-fallback-panel';
import { useT } from '@/i18n';
import { useOrganizationActions } from '@/hooks/organization/use-organization-actions';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import type { BillingDisplayCurrency } from '@/utils/billing-display';
import { DEFAULT_BILLING_DISPLAY, getBillingDisplaySettings } from '@/utils/billing-display';

export default function PricingSettingsPage() {
  return (
    <div className="mx-auto max-w-6xl space-y-4">
      <BillingDisplayPanel />
      <PricingFallbackPanel />
    </div>
  );
}

function BillingDisplayPanel() {
  const t = useT();
  const { currentOrganization, isLoading } = useOrganizations(true);
  const { updateOrganization, isUpdatingOrganization } = useOrganizationActions();
  const [currency, setCurrency] = useState<BillingDisplayCurrency>(
    DEFAULT_BILLING_DISPLAY.currency
  );
  const [usdToCnyRate, setUsdToCnyRate] = useState(String(DEFAULT_BILLING_DISPLAY.usdToCnyRate));
  const [rateError, setRateError] = useState('');
  const [savedVisible, setSavedVisible] = useState(false);
  const savedTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const canEdit = ['owner', 'admin'].includes(currentOrganization?.organization_role ?? '');
  const currentBillingDisplay = useMemo(
    () => getBillingDisplaySettings(currentOrganization),
    [currentOrganization?.billing_display_currency, currentOrganization?.usd_to_cny_rate]
  );
  const parsedRate = Number(usdToCnyRate.trim());
  const isRateInvalid =
    usdToCnyRate.trim() === '' || !Number.isFinite(parsedRate) || parsedRate <= 0;
  const isDirty =
    currency !== currentBillingDisplay.currency ||
    (!isRateInvalid && Math.abs(parsedRate - currentBillingDisplay.usdToCnyRate) > 0.000001);
  const isSaving = isUpdatingOrganization || isLoading;
  const isSaveDisabled = !currentOrganization || !canEdit || !isDirty || isRateInvalid || isSaving;

  useEffect(() => {
    const billingDisplay = getBillingDisplaySettings(currentOrganization);
    setCurrency(billingDisplay.currency);
    setUsdToCnyRate(String(billingDisplay.usdToCnyRate));
    setRateError('');
  }, [
    currentOrganization?.id,
    currentOrganization?.billing_display_currency,
    currentOrganization?.usd_to_cny_rate,
  ]);

  useEffect(() => {
    return () => {
      if (savedTimerRef.current) {
        clearTimeout(savedTimerRef.current);
      }
    };
  }, []);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setRateError('');
    if (!currentOrganization || !canEdit || !isDirty) return;
    if (isRateInvalid) {
      setRateError(t('settings.billingDisplay.rateInvalid'));
      return;
    }

    await updateOrganization({
      name: currentOrganization.name,
      billing_display_currency: currency,
      usd_to_cny_rate: parsedRate,
    });
    setSavedVisible(true);
    if (savedTimerRef.current) {
      clearTimeout(savedTimerRef.current);
    }
    savedTimerRef.current = setTimeout(() => setSavedVisible(false), 1800);
  };

  return (
    <form
      className="rounded-md border border-border bg-background p-5 shadow-sm"
      onSubmit={handleSubmit}
    >
      <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div className="min-w-0 flex-1 space-y-4">
          <div>
            <h3 className="text-sm font-medium">{t('settings.billingDisplay.title')}</h3>
            <p className="mt-1 text-sm text-muted-foreground">
              {t('settings.billingDisplay.description')}
            </p>
          </div>

          <div className="grid gap-4 sm:grid-cols-[minmax(0,260px)_minmax(0,260px)]">
            <div className="space-y-2">
              <Label className="text-xs font-semibold text-text-primary">
                {t('settings.billingDisplay.currency')}
              </Label>
              <div className="inline-flex h-9 overflow-hidden rounded-lg border border-border/80 bg-bg-canvas/40 p-0.5">
                {(['USD', 'CNY'] as const).map(nextCurrency => (
                  <button
                    key={nextCurrency}
                    type="button"
                    className={`rounded-md px-3 text-xs font-medium transition-colors ${
                      currency === nextCurrency
                        ? 'bg-background text-foreground shadow-sm'
                        : 'text-muted-foreground hover:text-foreground'
                    }`}
                    aria-pressed={currency === nextCurrency}
                    disabled={!canEdit || isSaving}
                    onClick={() => {
                      setCurrency(nextCurrency);
                      setSavedVisible(false);
                    }}
                  >
                    {nextCurrency === 'USD'
                      ? t('settings.billingDisplay.usd')
                      : t('settings.billingDisplay.cny')}
                  </button>
                ))}
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="billing-usd-to-cny-rate" className="text-xs font-semibold">
                {t('settings.billingDisplay.rate')}
              </Label>
              <Input
                id="billing-usd-to-cny-rate"
                type="number"
                min="0.0001"
                step="0.0001"
                value={usdToCnyRate}
                onChange={event => {
                  setUsdToCnyRate(event.target.value);
                  setSavedVisible(false);
                  if (rateError) setRateError('');
                }}
                disabled={!canEdit || isSaving}
                errorText={rateError}
                className="h-9 rounded-lg bg-bg-canvas/40 shadow-none transition-all focus:border-primary/50 focus:ring-0"
              />
              <p className="text-xs leading-5 text-muted-foreground">
                {t('settings.billingDisplay.rateHint')}
              </p>
            </div>
          </div>
        </div>

        <Button type="submit" size="sm" disabled={isSaveDisabled} className="h-9 gap-1.5">
          <Save className="size-3.5" />
          {isUpdatingOrganization
            ? t('settings.billingDisplay.saving')
            : savedVisible
              ? t('settings.billingDisplay.saved')
              : t('settings.billingDisplay.save')}
        </Button>
      </div>
    </form>
  );
}
