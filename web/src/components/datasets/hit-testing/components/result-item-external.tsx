'use client';

import React from 'react';
import { useT } from '@/i18n';
import { ExternalLink, Hash, Globe } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import { cn } from '@/lib/utils';
import type { ResultItemExternalProps } from '../types';
import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogClose,
  DialogBody,
} from '@/components/ui/dialog';

/**
 * ResultItemExternal Component
 * Displays a single hit testing result for external dataset
 */
export function ResultItemExternal({ result, index }: ResultItemExternalProps) {
  const t = useT('datasets');

  // Get score color based on relevance
  const getScoreColor = (score: number) => {
    if (score >= 0.8) return 'bg-success text-success-foreground';
    if (score >= 0.6) return 'bg-warning text-warning-foreground';
    return 'bg-destructive text-destructive-foreground';
  };

  // Format score percentage
  const scorePercentage = (result.score * 100).toFixed(1);

  // Extract source URI from metadata
  const sourceUri = result.metadata?.['x-amz-bedrock-kb-source-uri'];
  const dataSourceId = result.metadata?.['x-amz-bedrock-kb-data-source-id'];

  return (
    <Dialog>
      <DialogTrigger asChild>
        <Card className="min-w-0 overflow-hidden transition-colors hover:bg-muted/20 cursor-pointer">
          <CardContent className="p-4 space-y-3">
            {/* Result Header */}
            <div className="flex min-w-0 items-start justify-between gap-3">
              <div className="flex shrink-0 items-center gap-2">
                <Badge className={cn('px-2 py-1 text-xs', getScoreColor(result.score))}>
                  {scorePercentage}%
                </Badge>
                <div className="flex items-center gap-1 text-sm text-muted-foreground">
                  <Hash className="h-3 w-3" />
                  {index + 1}
                </div>
              </div>
              <div className="min-w-0 flex-1 text-right">
                <div className="flex items-center justify-end gap-1 text-sm font-medium [overflow-wrap:anywhere]">
                  <Globe className="h-3 w-3" />
                  {result.title}
                </div>
                {dataSourceId && (
                  <div className="text-xs text-muted-foreground [overflow-wrap:anywhere]">
                    {dataSourceId}
                  </div>
                )}
              </div>
            </div>
            {/* Result Content Preview */}
            <div className="line-clamp-3 overflow-hidden text-sm leading-relaxed text-foreground [overflow-wrap:anywhere]">
              {result.content}
            </div>
            {/* External Source Actions */}
            <div className="flex min-w-0 items-center justify-between gap-3 pt-2">
              <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
                <span>{t('hitTesting.externalSource')}</span>
                {sourceUri && (
                  <span className="truncate max-w-[200px] min-w-0" title={sourceUri}>
                    {sourceUri}
                  </span>
                )}
              </div>
              {sourceUri && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={event => {
                    event.preventDefault();
                    event.stopPropagation();
                    window.open(sourceUri, '_blank');
                  }}
                  className="h-6 px-2 text-xs"
                >
                  <ExternalLink className="h-3 w-3 mr-1" />
                  {t('hitTesting.viewSource')}
                </Button>
              )}
            </div>
            {/* Metadata */}
            <div className="flex items-center gap-4 text-xs text-muted-foreground">
              <span>{t('hitTesting.externalDataset')}</span>
              {result.metadata && Object.keys(result.metadata).length > 2 && (
                <span>
                  {t('hitTesting.additionalMetadata')}: {Object.keys(result.metadata).length - 2}
                </span>
              )}
            </div>
          </CardContent>
        </Card>
      </DialogTrigger>
      <DialogContent className="max-w-4xl p-0 overflow-hidden flex flex-col max-h-[90vh]">
        <DialogHeader>
          <DialogTitle className="text-xl font-bold tracking-tight">
            {t('hitTesting.details')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="py-6 space-y-6">
          <Card className="border-neutral-100 bg-neutral-50/50 rounded-2xl overflow-hidden shadow-sm transition-all hover:bg-white hover:shadow-md group">
            <CardContent className="p-6 space-y-6">
              {/* Result Header */}
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <Badge
                    className={cn(
                      'px-3 py-1 text-xs font-bold rounded-xl shadow-sm',
                      getScoreColor(result.score)
                    )}
                  >
                    {scorePercentage}%
                  </Badge>
                  <div className="flex items-center gap-1.5 text-sm font-bold text-neutral-400 uppercase tracking-wider bg-neutral-100/50 px-2.5 py-1 rounded-full">
                    <Hash className="h-3.5 w-3.5" />
                    {index + 1}
                  </div>
                </div>
                <div className="text-right space-y-1">
                  <div className="flex items-center justify-end gap-2 text-base font-bold text-neutral-800 tracking-tight">
                    <Globe className="h-4 w-4 text-blue-500" />
                    {result.title}
                  </div>
                  {dataSourceId && (
                    <div className="text-xs font-bold text-neutral-400 uppercase tracking-tight">
                      {dataSourceId}
                    </div>
                  )}
                </div>
              </div>

              <Separator className="bg-neutral-100" />

              {/* Result Content Full with Scroll */}
              <div className="text-sm leading-relaxed text-neutral-700 whitespace-pre-wrap max-h-80 overflow-y-auto pr-2 custom-scrollbar font-medium">
                {result.content}
              </div>

              <Separator className="bg-neutral-100" />

              {/* External Source Actions */}
              <div className="flex items-center justify-between pt-2">
                <div className="flex items-center gap-3">
                  <div className="flex items-center gap-2 text-xs font-bold text-neutral-400 uppercase tracking-wider bg-neutral-50 px-3 py-1.5 rounded-full border border-neutral-100">
                    <ExternalLink className="h-3.5 w-3.5" />
                    {t('hitTesting.externalSource')}
                  </div>
                  {sourceUri && (
                    <span
                      className="text-xs font-bold text-blue-600 truncate max-w-[240px]"
                      title={sourceUri}
                    >
                      {sourceUri}
                    </span>
                  )}
                </div>
                {sourceUri && (
                  <Button
                    variant="ghost"
                    onClick={event => {
                      event.preventDefault();
                      event.stopPropagation();
                      window.open(sourceUri, '_blank');
                    }}
                    className="h-10 px-4 text-sm font-bold text-blue-600 hover:text-blue-700 hover:bg-blue-50 rounded-xl transition-all active:scale-95 group"
                  >
                    {t('hitTesting.viewSource')}
                    <ExternalLink className="h-4 w-4 ml-2 transition-transform group-hover:translate-x-0.5 group-hover:-translate-y-0.5" />
                  </Button>
                )}
              </div>

              <Separator className="bg-neutral-100" />

              {/* Metadata */}
              <div className="flex items-center gap-4">
                <div className="text-xs font-bold text-neutral-500 uppercase flex items-center gap-2">
                  <span className="h-1.5 w-1.5 rounded-full bg-neutral-300" />
                  {t('hitTesting.externalDataset')}
                </div>
                {result.metadata && Object.keys(result.metadata).length > 2 && (
                  <div className="text-xs font-bold text-neutral-500 uppercase flex items-center gap-2">
                    <span className="h-1.5 w-1.5 rounded-full bg-neutral-300" />
                    {t('hitTesting.additionalMetadata')}: {Object.keys(result.metadata).length - 2}
                  </div>
                )}
              </div>
            </CardContent>
          </Card>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
          <DialogClose asChild>
            <Button variant="ghost" className="font-bold rounded-xl h-11 px-8 hover:bg-neutral-100">
              {t('hitTesting.close') || 'Close'}
            </Button>
          </DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
