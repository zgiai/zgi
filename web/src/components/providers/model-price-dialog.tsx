'use client';

import React, { useEffect, useMemo, useState } from 'react';
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
  const parsed = Number(trimmed);
  return !Number.isFinite(parsed) || parsed < 0;
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
    [currentOrganization?.billing_display_currency, currentOrganization?.usd_to_cny_rate]
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
  });

  const isImage = isImageGenerationModel(model?.use_cases);
  const isInputOnly = !isImage && isInputOnlyPriceModel(model?.use_cases);

  useEffect(() => {
    if (!model) {
      setValues({ inputPrice: '', outputPrice: '' });
      return;
    }

    if (isImage) {
      setValues({
        inputPrice: '',
        outputPrice: model.output_price_configured
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
      });
      return;
    }

    setValues({
      inputPrice: billingDisplayInputValueFromUSD(
        model.input_price,
        model.input_price_configured,
        billingDisplay
      ),
      outputPrice: isInputOnly
        ? ''
        : billingDisplayInputValueFromUSD(
            model.output_price,
            model.output_price_configured,
            billingDisplay
          ),
    });
  }, [billingDisplay, isImage, isInputOnly, model]);

  const errorText = useMemo(() => {
    if (priceValueInvalid(values.inputPrice) || priceValueInvalid(values.outputPrice)) {
      return t('aiProviders.models.priceDialog.invalidPrice');
    }
    return '';
  }, [t, values.inputPrice, values.outputPrice]);

  const handleSubmit = async () => {
    if (!model || errorText) return;

    await onSubmit({
      inputPrice: isImage ? '' : billingDisplayInputToUSD(values.inputPrice, billingDisplay),
      outputPrice:
        isImage || !isInputOnly ? billingDisplayInputToUSD(values.outputPrice, billingDisplay) : '',
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
                  step="0.0001"
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
                    step="0.0001"
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
                      step="0.0001"
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
            </div>
          )}

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
