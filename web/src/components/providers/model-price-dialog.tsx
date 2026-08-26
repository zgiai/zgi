'use client';

import React, { useEffect, useMemo, useState } from 'react';
import Decimal from 'decimal.js-light';
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
import { Label } from '@/components/ui/label';
import type { ModelItem } from '@/services/types/model';
import { useT } from '@/i18n';
import { isImageGenerationModel, isInputOnlyPriceModel } from '@/utils/model-price';
import {
  billingDisplayInputToUSD,
  billingDisplayInputValueFromUSD,
  getBillingCurrencySymbol,
  getBillingDisplaySettings,
} from '@/utils/billing-display';
import { useOrganizationStore } from '@/store/organization-store';

interface ModelPriceDialogValues {
  inputPrice: string;
  outputPrice: string;
  cacheReadPrice: string;
  cacheWritePrice: string;
}

interface ModelPriceDialogProps {
  open: boolean;
  model: ModelItem | null;
  onOpenChange: (open: boolean) => void;
  onSubmit: (values: ModelPriceDialogValues) => Promise<void>;
  isSubmitting?: boolean;
}

function priceValueInvalid(value: string): boolean {
  const trimmed = value.trim();
  if (trimmed === '') return false;
  try {
    return new Decimal(trimmed).isNegative();
  } catch {
    return true;
  }
}

export function ModelPriceDialog({
  open,
  model,
  onOpenChange,
  onSubmit,
  isSubmitting = false,
}: ModelPriceDialogProps): JSX.Element {
  const t = useT();
  const currentOrganization = useOrganizationStore.use.currentOrganization();
  const billingDisplay = useMemo(
    () => getBillingDisplaySettings(currentOrganization),
    [currentOrganization]
  );
  const currencySymbol = getBillingCurrencySymbol(billingDisplay);
  const perImageUnit = t(
    billingDisplay.currency === 'CNY'
      ? 'aiProviders.models.priceDialog.cnyPerImage'
      : 'aiProviders.models.priceDialog.usdPerImage'
  );
  const perMillionUnit = t(
    billingDisplay.currency === 'CNY'
      ? 'aiProviders.models.priceDialog.cnyPerMillion'
      : 'aiProviders.models.priceDialog.usdPerMillion'
  );
  const [values, setValues] = useState<ModelPriceDialogValues>({
    inputPrice: '',
    outputPrice: '',
    cacheReadPrice: '',
    cacheWritePrice: '',
  });

  const isImage = isImageGenerationModel(model?.use_cases);
  const isInputOnly = !isImage && isInputOnlyPriceModel(model?.use_cases);
  const isSyncedModel = model?.synced_input_price != null;

  useEffect(() => {
    if (!model) {
      setValues({ inputPrice: '', outputPrice: '', cacheReadPrice: '', cacheWritePrice: '' });
      return;
    }

    if (isImage) {
      setValues({
        inputPrice: '',
        outputPrice: isSyncedModel
          ? model.output_price_override == null
            ? ''
            : billingDisplayInputValueFromUSD(model.output_price_override, true, billingDisplay)
          : model.output_price_configured
            ? billingDisplayInputValueFromUSD(
                model.output_price,
                model.output_price_configured,
                billingDisplay
              )
            : billingDisplayInputValueFromUSD(
                model.input_price,
                model.input_price_configured,
                billingDisplay
              ),
        cacheReadPrice: '',
        cacheWritePrice: '',
      });
      return;
    }

    setValues({
      inputPrice: isSyncedModel
        ? model.input_price_override == null
          ? ''
          : billingDisplayInputValueFromUSD(model.input_price_override, true, billingDisplay)
        : billingDisplayInputValueFromUSD(
            model.input_price,
            model.input_price_configured,
            billingDisplay
          ),
      outputPrice: isInputOnly
        ? ''
        : isSyncedModel
          ? model.output_price_override == null
            ? ''
            : billingDisplayInputValueFromUSD(model.output_price_override, true, billingDisplay)
          : billingDisplayInputValueFromUSD(
              model.output_price,
              model.output_price_configured,
              billingDisplay
            ),
      cacheReadPrice: isSyncedModel
        ? model.cache_read_price_override == null
          ? ''
          : billingDisplayInputValueFromUSD(model.cache_read_price_override, true, billingDisplay)
        : billingDisplayInputValueFromUSD(
            model.cache_read_price,
            model.cache_read_price_configured,
            billingDisplay
          ),
      cacheWritePrice: isSyncedModel
        ? model.cache_write_price_override == null
          ? ''
          : billingDisplayInputValueFromUSD(model.cache_write_price_override, true, billingDisplay)
        : billingDisplayInputValueFromUSD(
            model.cache_write_price,
            model.cache_write_price_configured,
            billingDisplay
          ),
    });
  }, [billingDisplay, isImage, isInputOnly, isSyncedModel, model]);

  const errorText = useMemo(() => {
    if (
      priceValueInvalid(values.inputPrice) ||
      priceValueInvalid(values.outputPrice) ||
      priceValueInvalid(values.cacheReadPrice) ||
      priceValueInvalid(values.cacheWritePrice)
    ) {
      return t('aiProviders.models.priceDialog.invalidPrice');
    }
    return '';
  }, [t, values.cacheReadPrice, values.cacheWritePrice, values.inputPrice, values.outputPrice]);

  const handleSubmit = async () => {
    if (!model || errorText) return;

    await onSubmit({
      inputPrice: isImage ? '' : billingDisplayInputToUSD(values.inputPrice, billingDisplay),
      outputPrice:
        isImage || !isInputOnly ? billingDisplayInputToUSD(values.outputPrice, billingDisplay) : '',
      cacheReadPrice: isImage
        ? ''
        : billingDisplayInputToUSD(values.cacheReadPrice, billingDisplay),
      cacheWritePrice: isImage
        ? ''
        : billingDisplayInputToUSD(values.cacheWritePrice, billingDisplay),
    });
  };

  return (
    <Dialog open={open} onOpenChange={isSubmitting ? undefined : onOpenChange}>
      <DialogContent size="md">
        <DialogHeader>
          <DialogTitle>{t('aiProviders.models.priceDialog.title')}</DialogTitle>
          <DialogDescription>{model?.model || ''}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          {isImage ? (
            <div className="space-y-2">
              <Label htmlFor="model-image-price">
                {t('aiProviders.models.priceDialog.imagePrice')}
              </Label>
              <div className="relative">
                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
                  {currencySymbol}
                </span>
                <Input
                  id="model-image-price"
                  type="number"
                  min="0"
                  step="any"
                  className="pl-6 pr-16"
                  value={values.outputPrice}
                  onChange={event =>
                    setValues(current => ({ ...current, outputPrice: event.target.value }))
                  }
                />
                <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">
                  {perImageUnit}
                </span>
              </div>
            </div>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="model-input-price">
                  {t('aiProviders.models.fields.inputPrice')}
                </Label>
                <div className="relative">
                  <span className="absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
                    {currencySymbol}
                  </span>
                  <Input
                    id="model-input-price"
                    type="number"
                    min="0"
                    step="any"
                    className="pl-6 pr-20"
                    value={values.inputPrice}
                    onChange={event =>
                      setValues(current => ({ ...current, inputPrice: event.target.value }))
                    }
                  />
                  <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">
                    {perMillionUnit}
                  </span>
                </div>
              </div>

              {!isInputOnly && (
                <div className="space-y-2">
                  <Label htmlFor="model-output-price">
                    {t('aiProviders.models.fields.outputPrice')}
                  </Label>
                  <div className="relative">
                    <span className="absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
                      {currencySymbol}
                    </span>
                    <Input
                      id="model-output-price"
                      type="number"
                      min="0"
                      step="any"
                      className="pl-6 pr-20"
                      value={values.outputPrice}
                      onChange={event =>
                        setValues(current => ({ ...current, outputPrice: event.target.value }))
                      }
                    />
                    <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">
                      {perMillionUnit}
                    </span>
                  </div>
                </div>
              )}
              {isSyncedModel ? (
                <>
                  <PriceInput
                    id="model-cache-read-price"
                    label={t('aiProviders.models.fields.cacheReadPrice')}
                    value={values.cacheReadPrice}
                    onChange={cacheReadPrice =>
                      setValues(current => ({ ...current, cacheReadPrice }))
                    }
                    currencySymbol={currencySymbol}
                    unit={perMillionUnit}
                  />
                  <PriceInput
                    id="model-cache-write-price"
                    label={t('aiProviders.models.fields.cacheWritePrice')}
                    value={values.cacheWritePrice}
                    onChange={cacheWritePrice =>
                      setValues(current => ({ ...current, cacheWritePrice }))
                    }
                    currencySymbol={currencySymbol}
                    unit={perMillionUnit}
                  />
                </>
              ) : null}
            </div>
          )}

          {!isImage && isSyncedModel && model ? (
            <p className="text-xs text-muted-foreground">
              {t('aiProviders.models.priceDialog.syncedPrices', {
                input: model.synced_input_price ?? model.input_price,
                output: model.synced_output_price ?? model.output_price,
                cacheRead:
                  model.synced_cache_read_price ?? t('aiProviders.models.priceDialog.notProvided'),
                cacheWrite:
                  model.synced_cache_write_price ?? t('aiProviders.models.priceDialog.notProvided'),
              })}
            </p>
          ) : null}

          {errorText && <p className="text-xs text-destructive">{errorText}</p>}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isSubmitting}>
            {t('aiProviders.models.priceDialog.cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={!model || Boolean(errorText) || isSubmitting}>
            {isSubmitting
              ? t('aiProviders.models.priceDialog.saving')
              : t('aiProviders.models.priceDialog.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function PriceInput({
  id,
  label,
  value,
  onChange,
  currencySymbol,
  unit,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  currencySymbol: string;
  unit: string;
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      <div className="relative">
        <span className="absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
          {currencySymbol}
        </span>
        <Input
          id={id}
          type="number"
          min="0"
          step="any"
          className="pl-6 pr-20"
          value={value}
          onChange={event => onChange(event.target.value)}
        />
        <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">
          {unit}
        </span>
      </div>
    </div>
  );
}
