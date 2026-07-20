import React, { forwardRef, useImperativeHandle } from 'react';
import { useT } from '@/i18n';
import { Brain } from 'lucide-react';
import { cn } from '@/lib/utils';
import { ModelSelector } from '@/components/common/model-selector';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import { ModelFieldSection } from './model-field-section';
import { useAvailableModels } from '@/hooks/model/use-model';
import { useLocale } from '@/hooks/use-locale';
import { getModelDisplayName } from '@/utils/model-label';

interface EmbeddingSettingsProps {
  /** Currently selected embedding model in "provider/model" format */
  embeddingModel: ModelSelectorValue;
  /** Called when user picks a different model */
  onChange: (embeddingModel: ModelSelectorValue) => void;
  /** Disable the selector */
  disabled?: boolean;
  /** Whether the field is required */
  required?: boolean;
  /** Whether the selector should render in an error state */
  hasError?: boolean;
  /** Inline validation message */
  errorMessage?: string;
  /** Additional CSS classes */
  className?: string;
  /** Optional title override */
  title?: string;
  /** Optional description below the title */
  description?: string;
  /** Optional tooltip displayed beside the title */
  titleTooltip?: string;
  /** Optional placeholder override */
  placeholder?: string;
  /** Render the selected model as plain read-only text instead of a disabled selector */
  readOnlyDisplay?: boolean;
}

export interface EmbeddingSettingsRef {
  getFormData: () => ModelSelectorValue;
}

function ReadOnlyEmbeddingModelName({ embeddingModel }: { embeddingModel: ModelSelectorValue }) {
  const { locale } = useLocale();
  const { models } = useAvailableModels({ use_case: 'embedding' });
  const matchedModel = models.find(
    model => model.provider === embeddingModel.provider && model.model === embeddingModel.model
  );
  const displayName = matchedModel
    ? getModelDisplayName(matchedModel, locale)
    : embeddingModel.model || '-';

  return <div className="text-sm font-medium text-foreground">{displayName}</div>;
}

/**
 * @component EmbeddingSettings
 * @category Feature
 * @status Stable
 * @description Shared embedding model selector used across dataset creation and settings flows
 * @usage Render anywhere the dataset embedding model needs a consistent label, icon, and validation style
 * @example
 * <EmbeddingSettings embeddingModel={value} onChange={setValue} required />
 */
export const EmbeddingSettings = forwardRef<EmbeddingSettingsRef, EmbeddingSettingsProps>(
  (
    {
      embeddingModel,
      onChange,
      disabled = false,
      required = false,
      hasError = false,
      errorMessage,
      className,
      title,
      description,
      titleTooltip,
      placeholder,
      readOnlyDisplay = false,
    },
    ref
  ) => {
    const t = useT('datasets');

    // Expose getFormData method via ref
    useImperativeHandle(
      ref,
      () => ({
        getFormData: () => embeddingModel,
      }),
      [embeddingModel]
    );

    return (
      <ModelFieldSection
        icon={Brain}
        title={title || t('createWizard.processConfig.embeddingModel.title')}
        required={required}
        description={description}
        titleTooltip={titleTooltip}
        errorMessage={errorMessage}
        className={cn(className)}
      >
        {readOnlyDisplay ? (
          <ReadOnlyEmbeddingModelName embeddingModel={embeddingModel} />
        ) : (
          <ModelSelector
            modelType="embedding"
            value={{
              provider: embeddingModel.provider,
              model: embeddingModel.model,
            }}
            onChange={({ provider, model }) =>
              onChange({
                provider,
                model,
              })
            }
            placeholder={placeholder || t('createWizard.processConfig.embedding.selectModel')}
            disabled={disabled}
            hasError={hasError}
          />
        )}
      </ModelFieldSection>
    );
  }
);

EmbeddingSettings.displayName = 'EmbeddingSettings';
