'use client';

import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import type { ResultItemProps } from '../types';
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
import { Button } from '@/components/ui/button';
import Link from 'next/link';
import { useParams } from 'next/navigation';

const formatDebugNumber = (value?: number) =>
  typeof value === 'number' && Number.isFinite(value) ? value.toFixed(4) : '-';

const formatDebugRank = (value?: number) =>
  typeof value === 'number' && Number.isFinite(value) ? `#${value}` : '-';

export function ResultItem({ result, index }: ResultItemProps) {
  const t = useT('datasets');
  const { datasetId } = useParams<{ datasetId: string }>();
  const retrievalSource = result.retrieval_source;
  const sourceBadges = retrievalSource?.retrieval_sources ?? [];
  const matchedTerms = retrievalSource?.matched_terms ?? [];

  // Format score as percentage
  const scorePercentage = (result.score * 100).toFixed(1);
  const retrievalMethodLabel = retrievalSource ? t('hitTesting.retrievalMethod') : undefined;

  return (
    <Dialog>
      <DialogTrigger asChild>
        <Card className="mb-4 min-w-0 overflow-hidden transition hover:bg-muted/10 cursor-pointer">
          <CardHeader className="pb-3">
            <div className="flex min-w-0 items-center justify-between gap-3">
              <div className="flex shrink-0 items-center gap-2">
                <Badge variant="secondary">#{index + 1}</Badge>
                <Badge variant="outline" className="bg-green-50 text-green-700 border-green-200">
                  {scorePercentage}%
                </Badge>
              </div>
              {result.segment.document && (
                <div
                  className="min-w-0 flex-1 text-right text-sm text-muted-foreground [overflow-wrap:anywhere]"
                  title={result.segment.document.name}
                >
                  {result.segment.document.name}
                </div>
              )}
            </div>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="overflow-hidden text-sm leading-relaxed line-clamp-3 [overflow-wrap:anywhere]">
              {result.segment.content}
            </div>
            <Separator />
            {result.child_chunks && result.child_chunks.length > 0 && (
              <div className="space-y-2">
                <div className="text-xs font-medium text-muted-foreground">
                  {t('hitTesting.childChunks')}
                </div>
                <div className="max-h-32 overflow-y-auto space-y-2">
                  {result.child_chunks.map((chunk, idx) => (
                    <div key={chunk.id} className="bg-muted/50 p-2 rounded text-xs">
                      <div className="flex items-center justify-between mb-1">
                        <span className="font-medium">#{idx + 1}</span>
                        <span className="text-muted-foreground">
                          {(chunk.score * 100).toFixed(1)}%
                        </span>
                      </div>
                      <div className="text-muted-foreground [overflow-wrap:anywhere]">
                        {chunk.content}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}
            <Separator />
            {retrievalSource && (
              <>
                <div className="flex flex-wrap items-center gap-2 text-xs">
                  {retrievalMethodLabel && <Badge variant="outline">{retrievalMethodLabel}</Badge>}
                  {sourceBadges.map(source => (
                    <Badge key={source} variant="secondary" className="uppercase">
                      {source}
                    </Badge>
                  ))}
                  {retrievalSource.best_rank !== undefined && (
                    <span className="text-muted-foreground">
                      {t('hitTesting.bestRank')}: {formatDebugRank(retrievalSource.best_rank)}
                    </span>
                  )}
                </div>
                <Separator />
              </>
            )}
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
              <span>
                {t('hitTesting.position')}: {result.segment.position}
              </span>
              <span>
                {t('hitTesting.wordCount')}: {result.segment.word_count}
              </span>
              <span>|</span>
              <span>
                {t('hitTesting.tokens')}: {result.segment.tokens}
              </span>
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
          <Card className="border-neutral-100 bg-neutral-50/50 rounded-2xl overflow-hidden shadow-sm">
            <CardHeader className="pb-3 bg-white border-b border-neutral-100">
              <div className="flex min-w-0 items-center justify-between gap-4">
                <div className="flex shrink-0 items-center gap-3">
                  <Badge
                    variant="secondary"
                    className="bg-neutral-100 text-neutral-600 font-bold px-2.5 py-0.5 rounded-lg"
                  >
                    #{index + 1}
                  </Badge>
                  <Badge
                    variant="outline"
                    className="bg-emerald-50 text-emerald-700 border-emerald-100 font-bold px-2.5 py-0.5 rounded-lg shadow-sm"
                  >
                    {scorePercentage}%
                  </Badge>
                </div>
                {result.segment.document && (
                  <div
                    className="min-w-0 flex-1 text-right text-sm font-semibold text-neutral-500 [overflow-wrap:anywhere]"
                    title={result.segment.document.name}
                  >
                    {result.segment.document.name}
                  </div>
                )}
              </div>
            </CardHeader>
            <CardContent className="p-6 space-y-6 bg-white">
              {/* Full Segment Content */}
              <div className="text-sm leading-relaxed text-neutral-800 whitespace-pre-wrap font-medium [overflow-wrap:anywhere]">
                {result.segment.content}
              </div>

              <Separator className="bg-neutral-100" />

              {retrievalSource && (
                <>
                  <div className="space-y-4">
                    <div className="text-sm font-bold text-neutral-800 tracking-tight">
                      {t('hitTesting.retrievalDetails')}
                    </div>
                    <div className="grid gap-3 rounded-2xl border border-neutral-100 bg-neutral-50/50 p-4 sm:grid-cols-2 lg:grid-cols-3">
                      <div className="space-y-1">
                        <div className="text-[11px] font-bold uppercase text-neutral-400">
                          {t('hitTesting.retrievalMethod')}
                        </div>
                        <div className="text-sm font-semibold text-neutral-800">
                          {retrievalMethodLabel || retrievalSource.method}
                        </div>
                      </div>
                      <div className="space-y-1">
                        <div className="text-[11px] font-bold uppercase text-neutral-400">
                          {t('hitTesting.finalScore')}
                        </div>
                        <div className="text-sm font-semibold text-neutral-800">
                          {formatDebugNumber(retrievalSource.final_score ?? result.score)}
                        </div>
                      </div>
                      <div className="space-y-1">
                        <div className="text-[11px] font-bold uppercase text-neutral-400">
                          {t('hitTesting.bestRank')}
                        </div>
                        <div className="text-sm font-semibold text-neutral-800">
                          {formatDebugRank(retrievalSource.best_rank)}
                        </div>
                      </div>
                      <div className="space-y-1">
                        <div className="text-[11px] font-bold uppercase text-neutral-400">
                          {t('hitTesting.fusionScore')}
                        </div>
                        <div className="text-sm font-semibold text-neutral-800">
                          {formatDebugNumber(retrievalSource.fusion_score)}
                        </div>
                      </div>
                      <div className="space-y-1">
                        <div className="text-[11px] font-bold uppercase text-neutral-400">
                          {t('hitTesting.rerankScore')}
                        </div>
                        <div className="text-sm font-semibold text-neutral-800">
                          {formatDebugNumber(retrievalSource.rerank_score)}
                        </div>
                      </div>
                      <div className="space-y-1">
                        <div className="text-[11px] font-bold uppercase text-neutral-400">
                          {t('hitTesting.vectorScore')}
                        </div>
                        <div className="text-sm font-semibold text-neutral-800">
                          {formatDebugNumber(retrievalSource.vector_score)}
                        </div>
                      </div>
                      <div className="space-y-1">
                        <div className="text-[11px] font-bold uppercase text-neutral-400">
                          {t('hitTesting.bm25Score')}
                        </div>
                        <div className="text-sm font-semibold text-neutral-800">
                          {formatDebugNumber(retrievalSource.bm25_score)}
                        </div>
                      </div>
                      <div className="space-y-1">
                        <div className="text-[11px] font-bold uppercase text-neutral-400">
                          {t('hitTesting.vectorRank')} / {t('hitTesting.bm25Rank')}
                        </div>
                        <div className="text-sm font-semibold text-neutral-800">
                          {formatDebugRank(retrievalSource.vector_rank)} /{' '}
                          {formatDebugRank(retrievalSource.bm25_rank)}
                        </div>
                      </div>
                    </div>

                    {sourceBadges.length > 0 && (
                      <div className="space-y-2">
                        <div className="text-xs font-bold text-neutral-400 uppercase tracking-wider">
                          {t('hitTesting.retrievalSources')}
                        </div>
                        <div className="flex flex-wrap gap-2">
                          {sourceBadges.map(source => (
                            <Badge key={source} variant="secondary" className="uppercase">
                              {source}
                            </Badge>
                          ))}
                        </div>
                      </div>
                    )}

                    {matchedTerms.length > 0 && (
                      <div className="space-y-2">
                        <div className="text-xs font-bold text-neutral-400 uppercase tracking-wider">
                          {t('hitTesting.matchedTerms')}
                        </div>
                        <div className="flex flex-wrap gap-2">
                          {matchedTerms.map(term => (
                            <Badge key={term} variant="outline">
                              {term}
                            </Badge>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>

                  <Separator className="bg-neutral-100" />
                </>
              )}

              {result.child_chunks && result.child_chunks.length > 0 && (
                <div className="space-y-4">
                  <div className="text-xs font-bold text-neutral-400 uppercase tracking-wider">
                    {t('hitTesting.childChunks')}
                  </div>
                  <div className="space-y-3">
                    {result.child_chunks.map((chunk, idx) => (
                      <div
                        key={chunk.id}
                        className="bg-neutral-50 p-4 rounded-2xl border border-neutral-100 group transition-all hover:bg-white hover:shadow-md"
                      >
                        <div className="flex items-center justify-between mb-2">
                          <span className="text-xs font-bold text-neutral-500 uppercase">
                            Chunk #{idx + 1}
                          </span>
                          <Badge
                            variant="outline"
                            className="bg-blue-50 text-blue-700 border-blue-100 text-[10px] font-bold"
                          >
                            {(chunk.score * 100).toFixed(1)}%
                          </Badge>
                        </div>
                        <div className="text-sm text-neutral-600 whitespace-pre-wrap leading-relaxed [overflow-wrap:anywhere]">
                          {chunk.content}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              <Separator className="bg-neutral-100" />

              <div className="flex items-center gap-6 text-xs font-bold text-neutral-500">
                <div className="flex items-center gap-1.5 bg-neutral-50 px-3 py-1.5 rounded-full border border-neutral-100">
                  <span className="text-neutral-400 font-medium uppercase">
                    {t('hitTesting.wordCount')}:
                  </span>
                  <span className="text-neutral-700">{result.segment.word_count}</span>
                </div>
                <div className="flex items-center gap-1.5 bg-neutral-50 px-3 py-1.5 rounded-full border border-neutral-100">
                  <span className="text-neutral-400 font-medium uppercase">
                    {t('hitTesting.tokens')}:
                  </span>
                  <span className="text-neutral-700">{result.segment.tokens}</span>
                </div>
              </div>

              <Separator className="bg-neutral-100" />

              {/* Source File Details */}
              <div className="space-y-4">
                <div className="text-sm font-bold text-neutral-800 tracking-tight">
                  {t('hitTesting.fileDetails')}
                </div>
                <div className="bg-neutral-50/50 rounded-2xl border border-neutral-100 p-4 space-y-3">
                  <div className="flex items-start justify-between gap-4 text-sm">
                    <span className="shrink-0 text-neutral-500 font-medium">
                      {t('hitTesting.fileName')}:
                    </span>
                    <span className="min-w-0 text-right font-bold text-neutral-800 [overflow-wrap:anywhere]">
                      {result.segment.document.name}
                    </span>
                  </div>
                  {result.segment.document.doc_metadata && (
                    <div className="space-y-2 pt-2 border-t border-neutral-100">
                      {Object.entries(result.segment.document.doc_metadata).map(([key, value]) => (
                        <div key={key} className="flex items-start justify-between gap-4 text-[13px]">
                          <span className="shrink-0 text-neutral-500 capitalize">
                            {key.replace(/_/g, ' ')}:
                          </span>
                          <span className="min-w-0 text-right font-semibold text-neutral-700 [overflow-wrap:anywhere]">
                            {String(value)}
                          </span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                <Link
                  href={`/console/dataset/${datasetId}/documents/${result.segment.document.id}`}
                  className="inline-flex items-center gap-1.5 text-sm font-bold text-blue-600 hover:text-blue-700 transition-colors group"
                >
                  {t('hitTesting.viewDocumentDetails')}
                  <span className="text-lg transition-transform group-hover:translate-x-0.5">
                    →
                  </span>
                </Link>
              </div>
            </CardContent>
          </Card>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
          <DialogClose asChild>
            <Button variant="ghost" className="font-bold rounded-xl h-11 px-8 hover:bg-neutral-100">
              {t('hitTesting.close')}
            </Button>
          </DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
