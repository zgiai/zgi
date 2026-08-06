'use client';

import { AlertCircle, Check, Circle, Loader2 } from 'lucide-react';
import { Progress } from '@/components/ui/progress';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  GraphBuildStageKey,
  GraphBuildStageStatus,
  GraphDatasetStatus,
} from '@/services/types/dataset';

const stageOrder: GraphBuildStageKey[] = [
  'extraction',
  'alignment',
  'graph_sync',
  'vector_sync',
];

const fallbackStages: GraphBuildStageStatus[] = stageOrder.map(key => ({
  key,
  status: 'pending',
  progress: 0,
}));

interface GraphBuildProgressProps {
  status: GraphDatasetStatus;
}

export function GraphBuildProgress({ status }: GraphBuildProgressProps) {
  const t = useT('datasets');
  const stages = status.stages?.length ? status.stages : fallbackStages;
  const currentStage =
    status.current_stage || stages.find(stage => stage.status !== 'completed')?.key || 'vector_sync';

  return (
    <div className="flex flex-1 items-center justify-center p-6">
      <div className="w-full max-w-3xl rounded-2xl border border-border/80 bg-card p-6 text-left shadow-sm">
        <div className="flex items-start justify-between gap-6">
          <div className="flex min-w-0 items-start gap-3">
            <div className="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
              <Loader2 className="h-5 w-5 animate-spin" />
            </div>
            <div className="min-w-0">
              <h2 className="text-base font-semibold text-foreground">
                {t('graph.buildProgress.title')}
              </h2>
              <p className="mt-1 text-sm text-muted-foreground">
                {t('graph.buildProgress.currentStage', {
                  stage: t(`graph.buildProgress.stages.${currentStage}`),
                })}
              </p>
            </div>
          </div>
          <div className="shrink-0 text-right">
            <div className="text-2xl font-semibold tabular-nums text-foreground">
              {status.progress}%
            </div>
            <div className="text-xs text-muted-foreground">
              {t('graph.buildProgress.overall')}
            </div>
          </div>
        </div>

        <Progress value={status.progress} className="mt-5 h-2.5" />

        <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
          <span>
            {t('graph.buildProgress.documentSummary', {
              total: status.summary.documents_total,
              processing: status.summary.documents_processing,
            })}
          </span>
          {status.current_run ? (
            <span>
              {t('graph.buildProgress.runRevision', {
                revision: status.graph_revision,
              })}
            </span>
          ) : null}
        </div>

        <div className="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
          {stageOrder.map((key, index) => {
            const stage = stages.find(item => item.key === key) || fallbackStages[index];
            const isCurrent = key === currentStage && stage.status !== 'completed';
            const isCompleted = stage.status === 'completed';
            const isFailed = stage.status === 'failed';
            return (
              <div
                key={key}
                className={cn(
                  'rounded-xl border p-3 transition-colors',
                  isCurrent && 'border-primary/40 bg-primary/5',
                  isCompleted && 'border-emerald-200 bg-emerald-50/60',
                  isFailed && 'border-destructive/30 bg-destructive/5',
                  !isCurrent && !isCompleted && !isFailed && 'border-border/70 bg-muted/20'
                )}
              >
                <div className="flex items-center justify-between gap-2">
                  <div
                    className={cn(
                      'flex h-7 w-7 items-center justify-center rounded-full text-xs font-medium',
                      isCurrent && 'bg-primary text-primary-foreground',
                      isCompleted && 'bg-emerald-600 text-white',
                      isFailed && 'bg-destructive text-destructive-foreground',
                      !isCurrent && !isCompleted && !isFailed && 'bg-muted text-muted-foreground'
                    )}
                  >
                    {isCompleted ? (
                      <Check className="h-4 w-4" />
                    ) : isFailed ? (
                      <AlertCircle className="h-4 w-4" />
                    ) : isCurrent ? (
                      <Loader2 className="h-4 w-4 animate-spin" />
                    ) : (
                      <Circle className="h-3.5 w-3.5" />
                    )}
                  </div>
                  <span className="text-xs tabular-nums text-muted-foreground">
                    {stage.progress}%
                  </span>
                </div>
                <div className="mt-3 text-sm font-medium text-foreground">
                  {t(`graph.buildProgress.stages.${key}`)}
                </div>
                <div className="mt-0.5 text-xs text-muted-foreground">
                  {t(`graph.buildProgress.stageStatuses.${stage.status}`)}
                </div>
                <Progress value={stage.progress} className="mt-3 h-1.5" />
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
