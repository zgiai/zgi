'use client';

import { memo } from 'react';
import { Info } from 'lucide-react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { SelectItem } from '@/components/ui/select';
import { Badge } from '@/components/ui/badge';
import { ModelIcon } from 'modelicons';
import type { ModelItem } from '@/services/types/model';
import type { Locale } from '@/lib/i18n';
import type { FeatureLabels } from '../types';
import { serializeValue } from '../utils';
import { ModelTooltipContent } from '@/components/model/model-tooltip-content';
import { getModelDisplayName } from '@/utils/model-label';

export interface ModelRowItemProps {
  model: ModelItem;
  providerId: string;
  contextLabel: string;
  deprecatedUnavailableLabel: string;
  featuresLabel: string;
  replacementSuggestionLabel: string;
  useCaseLabel: string;
  featureLabels: FeatureLabels;
  useCaseLabels: Record<string, string>;
  locale: Locale;
  highlighted?: boolean;
  highlightedLabel?: string;
}

// Model row item component with optional tooltip
export const ModelRowItem = memo(function ModelRowItem({
  model,
  providerId,
  contextLabel,
  deprecatedUnavailableLabel,
  featuresLabel,
  replacementSuggestionLabel,
  useCaseLabel,
  featureLabels,
  useCaseLabels,
  locale,
  highlighted = false,
  highlightedLabel,
}: ModelRowItemProps) {
  const modelLabel = getModelDisplayName(model, locale);
  const hasFeatures = Object.values(model.features || {}).some(Boolean);
  const hasMeta =
    (model.context_window !== undefined && model.context_window > 0) ||
    (model.use_cases && model.use_cases.length > 0);
  const shouldShowTooltip = hasFeatures || hasMeta;

  const itemNode = (
    <SelectItem
      value={serializeValue({ provider: providerId, model: model.model })}
      className="h-9 cursor-pointer group"
      onMouseDown={e => {
        e.preventDefault();
      }}
    >
      <div className="flex items-center gap-2 w-full">
        <ModelIcon
          model={model.model}
          className="shrink-0 flex items-center justify-center"
          size={20}
        />
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-xs truncate">{modelLabel}</span>
            {highlighted && highlightedLabel ? (
              <Badge className="h-4 px-1.5 py-0 text-[10px] leading-4">{highlightedLabel}</Badge>
            ) : null}
            {shouldShowTooltip && (
              <Info className="h-3 w-3 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity" />
            )}
          </div>
        </div>
      </div>
    </SelectItem>
  );

  if (!shouldShowTooltip) {
    return itemNode;
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>{itemNode}</TooltipTrigger>
      <TooltipContent side="right" className="p-3">
        <ModelTooltipContent
          model={model}
          labels={{
            context: contextLabel,
            deprecatedUnavailable: deprecatedUnavailableLabel,
            features: featuresLabel,
            replacementSuggestion: replacementSuggestionLabel,
            useCases: useCaseLabel,
          }}
          featureLabels={featureLabels}
          useCaseLabels={useCaseLabels}
        />
      </TooltipContent>
    </Tooltip>
  );
});
