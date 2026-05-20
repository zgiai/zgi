'use client';

import { useT, type AllTranslationKeys } from '@/i18n';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import type {
  DocumentExtractionStrategy,
  DocumentExtractionStrategyStatus,
} from '@/services/types/dataset';
import { Badge } from '@/components/ui/badge';

interface ExtractionStrategySelectProps {
  value?: DocumentExtractionStrategy;
  strategies: DocumentExtractionStrategy[];
  items?: DocumentExtractionStrategyStatus[];
  recommendedStrategy?: DocumentExtractionStrategy;
  loading?: boolean;
  disabled?: boolean;
  onChange: (value: DocumentExtractionStrategy) => void;
}

export function extractionStrategyLabelKey(
  strategy?: DocumentExtractionStrategy
): AllTranslationKeys | undefined {
  return strategy
    ? (`datasets.documents.extractionStrategy.options.${strategy}` as AllTranslationKeys)
    : undefined;
}

export function ExtractionStrategySelect({
  value,
  strategies,
  items,
  recommendedStrategy,
  loading = false,
  disabled = false,
  onChange,
}: ExtractionStrategySelectProps) {
  const t = useT();
  const options =
    items && items.length > 0
      ? items
      : strategies.map(strategy => ({
          strategy,
          available: true,
          configured: true,
          recommended: strategy === recommendedStrategy,
        }));

  return (
    <div className="flex items-center gap-2">
      <span className="text-sm text-muted-foreground whitespace-nowrap">
        {t('datasets.documents.extractionStrategy.label')}
      </span>
      <Select
        value={value}
        onValueChange={next => onChange(next as DocumentExtractionStrategy)}
        disabled={disabled || loading || strategies.length === 0}
      >
        <SelectTrigger isLoading={loading} className="h-9 w-[190px]">
          <SelectValue placeholder={t('datasets.documents.extractionStrategy.empty')} />
        </SelectTrigger>
        <SelectContent>
          {options.map(item => (
            <SelectItem key={item.strategy} value={item.strategy} disabled={!item.available}>
              <span className="flex w-full items-center justify-between gap-3">
                <span>
                  {t(
                    extractionStrategyLabelKey(item.strategy) ??
                      'datasets.documents.extractionStrategy.empty'
                  )}
                </span>
                {item.recommended || item.strategy === recommendedStrategy ? (
                  <Badge variant="secondary" className="h-5 rounded-full px-2 text-[11px]">
                    {t('datasets.documents.extractionStrategy.recommended')}
                  </Badge>
                ) : null}
                {!item.available ? (
                  <span className="text-xs text-muted-foreground">
                    {t('datasets.documents.extractionStrategy.unavailable')}
                  </span>
                ) : null}
              </span>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
