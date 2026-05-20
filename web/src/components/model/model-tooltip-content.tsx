'use client';

import { memo } from 'react';
import { Cpu, Sparkles, LayoutGrid } from 'lucide-react';
import { ModelIcon } from '@lobehub/icons';
import type { ModelItem, ModelUseCase } from '@/services/types/model';
import { ModelFeatureIcon } from '@/components/model/model-feature-icon';
import { ModelUseCaseIcon } from '@/components/model/model-use-case-icon';
import { formatTokens } from '@/utils/format';
import { cn } from '@/lib/utils';
import { USE_CASE_BADGE_COLORS } from '@/config/model-colors';

// Priority order for displaying features (most important first)
const FEATURE_PRIORITY = [
  'function_calling',
  'reasoning',
  'structured_output',
  'vision',
  'web_search',
  'code_interpreter',
  'file_search',
  'attachment',
  'json_mode',
  'streaming',
  'system_prompt',
];

export interface ModelTooltipContentProps {
  model: ModelItem;
  labels: {
    context: string;
    features: string;
    useCases: string;
  };
  featureLabels: Record<string, string>;
  useCaseLabels: Record<string, string>;
}

export const ModelTooltipContent = memo(function ModelTooltipContent({
  model,
  labels,
  featureLabels,
  useCaseLabels,
}: ModelTooltipContentProps) {
  const modelLabel = model.model_name || model.model;

  // Collect enabled features from the features object, sorted by priority
  const enabledFeatures = Object.entries(model.features || {})
    .filter(([, enabled]) => enabled)
    .sort(([a], [b]) => {
      const aIndex = FEATURE_PRIORITY.indexOf(a);
      const bIndex = FEATURE_PRIORITY.indexOf(b);
      if (aIndex === -1 && bIndex === -1) return 0;
      if (aIndex === -1) return 1;
      if (bIndex === -1) return -1;
      return aIndex - bIndex;
    })
    .slice(0, 8)
    .map(([key]) => ({ key, label: featureLabels[key] || key.replace(/_/g, ' ') }));

  const useCases = (model.use_cases || []).map(uc => ({
    key: uc,
    label: useCaseLabels[uc] || uc,
  }));

  const hasContext = model.context_window !== undefined && model.context_window > 0;
  const hasFeatures = enabledFeatures.length > 0;
  const hasUseCases = useCases.length > 0;

  return (
    <div className="min-w-[280px] space-y-2">
      {/* Model name header */}
      <div className="flex items-center gap-2 w-full">
        <ModelIcon model={model.model} size={24} className="text-muted-foreground" />
        <div className="w-0 grow">
          <div className="font-medium text-sm truncate w-full">{modelLabel}</div>
          <div className="text-xs !mt-0 truncate w-full">{model.model}</div>
        </div>
      </div>

      {/* Context window */}
      {hasContext && (
        <div className="flex items-center gap-1.5 text-xs">
          <Cpu className="h-3.5 w-3.5" />
          <span>{labels.context}</span>
          <span className="font-medium text-foreground">{formatTokens(model.context_window)}</span>
        </div>
      )}

      {/* Use Cases grid */}
      {hasUseCases && (
        <div className="space-y-1.5">
          <div className="flex items-center gap-1.5 text-xs font-medium">
            <LayoutGrid className="h-3.5 w-3.5" />
            <span>{labels.useCases}</span>
          </div>
          <div className="grid grid-cols-2 gap-1.5">
            {useCases.map(uc => (
              <div
                key={uc.key}
                className={cn(
                  'flex items-center gap-1.5 text-xs rounded-md px-2 py-1 border',
                  USE_CASE_BADGE_COLORS[uc.key as ModelUseCase] || 'bg-accent/50'
                )}
              >
                <ModelUseCaseIcon useCase={uc.key} className="h-3 w-3 shrink-0" colored />
                <span className="truncate text-[10px] font-medium leading-none">{uc.label}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Features grid */}
      {hasFeatures && (
        <div className="space-y-1.5">
          <div className="flex items-center gap-1.5 text-xs font-medium">
            <Sparkles className="h-3.5 w-3.5" />
            <span>{labels.features}</span>
          </div>
          <div className="grid grid-cols-2 gap-1.5">
            {enabledFeatures.map(f => (
              <div
                key={f.key}
                className="flex items-center gap-1.5 text-xs bg-background border border-border rounded-md px-2 py-1"
                title={f.label}
              >
                <ModelFeatureIcon feature={f.key} className="h-3 w-3 shrink-0" />
                <span className="truncate text-[10px] leading-none text-foreground">{f.label}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
});
