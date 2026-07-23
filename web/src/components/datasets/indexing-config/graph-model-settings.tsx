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
  placeholder?: string;
  embeddingModel?: ModelSelectorValue;
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
  placeholder,
  embeddingModel,
}: GraphModelSettingsProps) {
  const t = useT('datasets');

  return (
    <div className="space-y-3">
      <ModelFieldSection
        icon={Network}
        title={title || t('createWizard.processConfig.graphModel.title')}
        required={required}
        description={description}
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
      {embeddingModel ? (
        <div className="rounded-md border border-border/70 bg-muted/30 px-3 py-2">
          <div className="text-xs font-medium text-foreground">
            {t('graph.inheritedEmbeddingModel')}
          </div>
          <div className="mt-1 text-xs text-muted-foreground">
            {embeddingModel.provider} / {embeddingModel.model}
          </div>
        </div>
      ) : null}
    </div>
  );
}
