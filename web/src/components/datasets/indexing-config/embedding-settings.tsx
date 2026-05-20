import React, { forwardRef, useImperativeHandle } from 'react';
import { useT } from '@/i18n';
import { Brain } from 'lucide-react';
import { cn } from '@/lib/utils';
import { ModelSelector } from '@/components/common/model-selector';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import { ModelFieldSection } from './model-field-section';

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
  /** Optional placeholder override */
  placeholder?: string;
}

export interface EmbeddingSettingsRef {
  getFormData: () => ModelSelectorValue;
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
      placeholder,
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
        errorMessage={errorMessage}
        className={cn(className)}
      >
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
      </ModelFieldSection>
    );
  }
);

EmbeddingSettings.displayName = 'EmbeddingSettings';
