'use client';

import { AlertTriangle } from 'lucide-react';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import type { DocumentExtractionMetadata } from '@/services/types/dataset';
import { extractionStrategyLabelKey } from './extraction-strategy-select';

interface ExtractionFallbackNoticeProps {
  extraction?: DocumentExtractionMetadata | null;
  variant?: 'badge' | 'banner';
}

export function ExtractionFallbackNotice({
  extraction,
  variant = 'badge',
}: ExtractionFallbackNoticeProps) {
  const t = useT();

  if (!extraction?.fallback_used) return null;

  const requestedKey = extractionStrategyLabelKey(extraction.requested_strategy);
  const actualKey = extractionStrategyLabelKey(extraction.actual_strategy);
  const requested = requestedKey ? t(requestedKey) : '';
  const actual = actualKey ? t(actualKey) : '';
  const message = t('datasets.documents.extractionStrategy.fallbackTooltip', {
    requested,
    actual,
  });

  if (variant === 'banner') {
    return (
      <div className="flex items-center gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-200">
        <AlertTriangle className="h-4 w-4 shrink-0" />
        <span>{message}</span>
      </div>
    );
  }

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge
            variant="outline"
            className="shrink-0 border-amber-300 bg-amber-50 text-amber-700 hover:bg-amber-50"
            onClick={event => event.stopPropagation()}
          >
            <AlertTriangle className="mr-1 h-3 w-3" />
            {t('datasets.documents.extractionStrategy.fallbackBadge')}
          </Badge>
        </TooltipTrigger>
        <TooltipContent className="max-w-xs">{message}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
