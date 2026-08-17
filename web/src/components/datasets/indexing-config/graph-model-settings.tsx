import { Network } from 'lucide-react';
import { useT } from '@/i18n';
import { ModelSelector } from '@/components/common/model-selector';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import { ModelFieldSection } from './model-field-section';

interface GraphModelSettingsProps {
  graphModel: ModelSelectorValue;
  onChange: (graphModel: ModelSelectorValue) => void;
  disabled?: boolean;
  required?: boolean;
  hasError?: boolean;
  errorMessage?: string;
  className?: string;
  title?: string;
  description?: string;
  titleTooltip?: string;
  placeholder?: string;
}

/**
 * @component GraphModelSettings
 * @category Feature
 * @status Stable
 * @description GraphFlow entity extraction model selector backed by the text-chat model pool
 * @usage Render when dataset graph flow is enabled so users can choose the entity extraction model
 * @example
 * <GraphModelSettings graphModel={value} onChange={setValue} required />
 */
export function GraphModelSettings({
  graphModel,
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
}: GraphModelSettingsProps) {
  const t = useT('datasets');

  return (
    <div className="space-y-3">
      <ModelFieldSection
        icon={Network}
        title={title || t('createWizard.processConfig.graphModel.title')}
        required={required}
        description={description}
        titleTooltip={titleTooltip || t('createWizard.processConfig.graphModel.help')}
        errorMessage={errorMessage}
        className={className}
      >
        <ModelSelector
          modelType="text-chat"
          value={{
            provider: graphModel.provider,
            model: graphModel.model,
          }}
          onChange={({ provider, model }) =>
            onChange({
              provider,
              model,
            })
          }
          placeholder={placeholder || t('createWizard.processConfig.graphModel.placeholder')}
          disabled={disabled}
          hasError={hasError}
        />
      </ModelFieldSection>
    </div>
  );
}
