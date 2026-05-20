'use client';

import React, { useCallback, useRef, useEffect } from 'react';
import { useT } from '@/i18n';
import { Search, Sparkles } from 'lucide-react';
import { Button } from '@/components/ui/button';
// import Link from 'next/link';
// import { useParams } from 'next/navigation';
import { Textarea } from '@/components/ui/textarea';
import { cn } from '@/lib/utils';
import type { QueryTextareaProps } from '../types';

/**
 * QueryTextarea Component
 * Handles query input and submission with configuration display
 * Supports Enter to submit, Shift+Enter for new line
 */
export function QueryTextarea({
  query,
  onQueryChange,
  onSubmit,
  isLoading,
  maxLength = 200,
  onConfigChange,
}: QueryTextareaProps) {
  const t = useT('datasets');
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  // const { datasetId } = useParams<{ datasetId: string }>();

  // Handle keyboard shortcuts
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        if (!isLoading && query.trim()) {
          onSubmit();
        }
      }
    },
    [isLoading, query, onSubmit]
  );

  // Auto-focus on mount
  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.focus();
    }
  }, []);

  // Handle submit
  const handleSubmit = useCallback(() => {
    if (!query.trim()) return;
    onSubmit();
  }, [query, onSubmit]);

  // Get remaining characters
  const remainingChars = maxLength - query.length;
  const isOverLimit = remainingChars < 0;

  return (
    <div className="flex h-full flex-col rounded-xl border bg-card shadow-sm">
      {/* Configuration Summary */}
      <div className="border-b px-4 py-3">
        <div className="flex items-center justify-between gap-3">
          <h2 className="text-sm font-semibold text-foreground">{t('hitTesting.testText')}</h2>
          <div>
            {/* TODO: Batch testing feature temporarily hidden
            <Link href={`/console/dataset/${datasetId}/batch-testing`} prefetch>
              <Button variant="outline" className="h-6 px-2 text-xs mr-2">
                {t('hitTesting.batchTesting')}
              </Button>
            </Link>
            */}
            <Button onClick={onConfigChange} variant="outline" className="h-8 px-3 text-xs">
              {t('hitTesting.modify')}
            </Button>
          </div>
        </div>
        {/* <div className="flex flex-wrap gap-2 bg-muted/50 rounded-lg">
            <Badge variant="outline" className="text-xs">
              {t(`hitTesting.methods.${retrievalConfig.search_method}`)}
            </Badge>
            <Badge variant="outline" className="text-xs">
              Top-K: {retrievalConfig.top_k}
            </Badge>
            {retrievalConfig.score_threshold_enabled && (
              <Badge variant="outline" className="text-xs">
                {t('hitTesting.threshold')}: {retrievalConfig.score_threshold}
              </Badge>
            )}
            {retrievalConfig.reranking_enable && (
              <Badge variant="outline" className="bg-primary/10 text-primary text-xs">
                {t('hitTesting.rerankingEnabled')}
              </Badge>
            )}
          </div> */}
      </div>

      {/* Query Input */}
      <div className="flex min-h-0 flex-1 flex-col p-4">
        {/* <div className="flex items-center justify-between">
          <Label htmlFor="query-textarea">{t('hitTesting.queryLabel')}</Label>
          <span
            className={cn('text-xs', isOverLimit ? 'text-destructive' : 'text-muted-foreground')}
          >
            {remainingChars} {tc('characters')}
          </span>
        </div> */}
        <div className="min-h-0 flex-1">
          <Textarea
            ref={textareaRef}
            id="query-textarea"
            value={query}
            onChange={e => onQueryChange(e.target.value)}
            placeholder={t('hitTesting.queryPlaceholder')}
            onKeyDown={handleKeyDown}
            className={cn(
              'h-full min-h-0 resize-none border-border/80 bg-background text-sm leading-6',
              isOverLimit && 'border-destructive focus:border-destructive'
            )}
            maxLength={maxLength}
            disabled={isLoading}
          />
        </div>

        <div className="mt-3 flex items-center justify-between gap-3">
          <div
            className={cn(
              'min-w-0 text-xs text-muted-foreground',
              isOverLimit && 'text-destructive'
            )}
          >
            {t('hitTesting.inputHelpText')} · {query.length}/{maxLength}
          </div>
          <Button
            size="sm"
            onClick={handleSubmit}
            disabled={isLoading || !query.trim() || isOverLimit}
            className="h-8 shrink-0 px-3"
          >
            {isLoading ? (
              <>
                <Sparkles className="mr-1 h-3 w-3 animate-spin" />
                {t('hitTesting.searching')}
              </>
            ) : (
              <>
                <Search className="mr-1 h-3 w-3" />
                {t('hitTesting.search')}
              </>
            )}
          </Button>
        </div>
      </div>
    </div>
  );
}
