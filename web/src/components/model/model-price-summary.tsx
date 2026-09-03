'use client';

import { memo } from 'react';
import { Coins } from 'lucide-react';
import type { ModelItem } from '@/services/types/model';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import { useOrganizationStore } from '@/store/organization-store';
import { getBillingDisplaySettings } from '@/utils/billing-display';
import { getModelPriceDisplay, type ModelPriceDisplayItem } from '@/utils/model-price';

interface ModelPriceSummaryProps {
  model: ModelItem;
  className?: string;
}

export function hasModelPriceDisplay(model: ModelItem): boolean {
  return Boolean(
    model.pricing ||
      model.currency ||
      model.input_price_configured ||
      model.output_price_configured ||
      typeof model.input_price === 'number' ||
      typeof model.output_price === 'number'
  );
}

export const ModelPriceSummary = memo(function ModelPriceSummary({
  model,
  className,
}: ModelPriceSummaryProps) {
  const t = useT('models');
  const currentOrganization = useOrganizationStore.use.currentOrganization();
  const billingDisplay = getBillingDisplaySettings(currentOrganization);

  const priceItems = getModelPriceDisplay({
    inputPrice: model.input_price,
    outputPrice: model.output_price,
    inputPriceConfigured: model.input_price_configured,
    outputPriceConfigured: model.output_price_configured,
    useCases: model.use_cases,
    currency: model.currency,
    pricing: model.pricing,
    billingDisplay,
    videoDisplayMode: 'summary',
    labels: {
      withVideoInput: t('plaza.withVideoInput'),
      withoutVideoInput: t('plaza.withoutVideoInput'),
      image: t('plaza.image'),
      input: t('plaza.input'),
      output: t('plaza.output'),
      speechGeneration: t('plaza.speechGeneration'),
      transcription: t('plaza.transcription'),
      musicGeneration: t('plaza.musicGeneration'),
      lyricsGeneration: t('plaza.lyricsGeneration'),
      meteredPrice: t('plaza.meteredPrice'),
      perImage: t('plaza.perImage'),
      perMillionTokens: t('plaza.perMillionTokens'),
      perTenThousandCharacters: t('plaza.perTenThousandCharacters'),
      perHour: t('plaza.perHour'),
      perTrack: t('plaza.perTrack'),
      perRequest: t('plaza.perRequest'),
      perQuantity: (quantity, unit) => t('plaza.perQuantity', { quantity, unit }),
      perUnit: unit => t('plaza.perUnit', { unit }),
    },
  });

  if (priceItems.length === 0) {
    return null;
  }

  return (
    <div className={cn('space-y-1.5', className)}>
      <div className="flex items-center gap-1.5 text-xs font-medium">
        <Coins className="h-3.5 w-3.5" />
        <span>{t('selector.tooltip.pricing')}</span>
      </div>
      <div className={cn('grid gap-1.5', priceItems.length > 1 ? 'grid-cols-2' : 'grid-cols-1')}>
        {priceItems.map((item, index) => (
          <ModelPriceChip
            key={`${item.label}-${item.detail ?? item.unit}-${index}`}
            item={item}
            labels={{
              image: t('plaza.image'),
              input: t('plaza.input'),
              output: t('plaza.output'),
              video: t('plaza.video'),
              perImage: t('plaza.perImage'),
              perSecond: t('plaza.perSecond'),
              perTask: t('plaza.perTask'),
              perMillionVideoTokens: t('plaza.perMillionVideoTokens'),
              perMillionTokens: t('plaza.perMillionTokens'),
            }}
          />
        ))}
      </div>
    </div>
  );
});

function ModelPriceChip({
  item,
  labels,
}: {
  item: ModelPriceDisplayItem;
  labels: {
    image: string;
    input: string;
    output: string;
    video: string;
    perImage: string;
    perSecond: string;
    perTask: string;
    perMillionVideoTokens: string;
    perMillionTokens: string;
  };
}) {
  const label =
    item.detail ||
    (item.unit === 'perImage'
      ? labels.image
      : item.label === 'input'
        ? labels.input
        : item.label === 'output'
          ? labels.output
          : labels.video);
  const unit =
    item.unit === 'perImage'
      ? labels.perImage
      : item.unit === 'perSecond'
        ? labels.perSecond
        : item.unit === 'perTask'
          ? labels.perTask
          : item.displayUnit ||
            (item.unit === 'perMillionVideoTokens'
              ? labels.perMillionVideoTokens
              : labels.perMillionTokens);

  return (
    <div
      className="min-w-0 rounded-md border border-border bg-background px-2 py-1.5"
      title={`${label} ${item.formattedValue} ${unit}`}
    >
      <div className="truncate text-[10px] leading-none text-muted-foreground">{label}</div>
      <div className="mt-1 truncate text-xs font-semibold leading-none text-foreground tabular-nums">
        {item.formattedValue}
      </div>
      <div className="mt-1 truncate text-[10px] leading-none text-muted-foreground">{unit}</div>
    </div>
  );
}
