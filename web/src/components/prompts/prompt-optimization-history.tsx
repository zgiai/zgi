'use client';

import { useMemo } from 'react';
import { Copy, History, Sparkles } from 'lucide-react';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Textarea } from '@/components/ui/textarea';
import {
  useAdoptPromptOptimizationRun,
  usePromptOptimizationRuns,
} from '@/hooks/prompt/use-prompts';
import type {
  PromptOptimizationRun,
  PromptSource,
  PromptVersion,
} from '@/services/types/prompt';

interface PromptOptimizationHistoryProps {
  promptId: string;
  promptSource: PromptSource;
  promptVersions: PromptVersion[];
  canManage: boolean;
  onRetryRun?: (run: PromptOptimizationRun) => void;
}

export function PromptOptimizationHistory({
  promptId,
  promptSource,
  promptVersions,
  canManage,
  onRetryRun,
}: PromptOptimizationHistoryProps) {
  const t = useT('prompts');
  const { runs, isLoading, error } = usePromptOptimizationRuns(promptId, { limit: 10 }, true);
  const adoptRun = useAdoptPromptOptimizationRun(promptId);

  const versionIndex = useMemo(() => {
    return new Map(promptVersions.map(version => [version.id, version.version]));
  }, [promptVersions]);

  const canAdopt = canManage && promptSource !== 'official';

  const getGoalLabel = (goal: string) => {
    switch (goal) {
      case 'general':
      case 'reliable':
      case 'structured':
      case 'deep':
        return t(`optimizer.goals.${goal}.label`);
      case 'professional':
        return t('optimizer.goals.structured.label');
      case 'json':
        return t('optimizer.goals.reliable.label');
      default:
        return goal;
    }
  };

  const handleCopy = async (run: PromptOptimizationRun) => {
    const text = run.output;
    try {
      await navigator.clipboard.writeText(text);
      toast.success(t('messages.optimizerCopied'));
    } catch {
      toast.error(t('messages.optimizerCopyFailed'));
    }
  };

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center gap-2">
          <History className="h-5 w-5 text-muted-foreground" />
          <CardTitle className="text-lg">{t('history.title')}</CardTitle>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="text-sm text-muted-foreground">{t('history.description')}</div>

        {isLoading ? (
          <div className="text-sm text-muted-foreground">{t('states.loading')}</div>
        ) : error ? (
          <div className="text-sm text-destructive">{error}</div>
        ) : runs.length === 0 ? (
          <div className="rounded-lg border border-dashed p-6 text-sm text-muted-foreground">
            {t('history.empty')}
          </div>
        ) : (
          <div className="space-y-4">
            {runs.map(run => {
              const adoptedVersionNumber = run.adopted_prompt_version_id
                ? versionIndex.get(run.adopted_prompt_version_id)
                : undefined;

              return (
                <div key={run.id} className="rounded-xl border p-4 space-y-4">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge variant="outline">{new Date(run.created_at).toLocaleString()}</Badge>
                      <Badge variant="secondary">{getGoalLabel(run.goal)}</Badge>
                      {run.provider && run.model ? (
                        <Badge variant="outline">
                          {run.provider} / {run.model}
                        </Badge>
                      ) : null}
                      {run.preserve_variables ? (
                        <Badge variant="outline">{t('history.variablesProtected')}</Badge>
                      ) : null}
                      {run.adopted_variant ? (
                        <Badge variant="default">
                          {adoptedVersionNumber
                            ? t('history.adoptedVersion', { version: adoptedVersionNumber })
                            : t('history.adopted')}
                        </Badge>
                      ) : null}
                    </div>
                    <div className="flex items-center gap-2">
                      <Button variant="outline" onClick={() => void handleCopy(run)}>
                        <Copy className="h-4 w-4" />
                        {t('history.copyCurrent')}
                      </Button>
                      {onRetryRun ? (
                        <Button variant="outline" onClick={() => onRetryRun(run)}>
                          {t('history.retryCurrent')}
                        </Button>
                      ) : null}
                      {canAdopt ? (
                        <Button
                          onClick={() =>
                            adoptRun.mutate({
                              runId: run.id,
                              payload: { variant: 'balanced' },
                            })
                          }
                          disabled={adoptRun.isPending || Boolean(run.adopted_variant)}
                        >
                          <Sparkles className="h-4 w-4" />
                          {run.adopted_variant ? t('history.adopted') : t('history.adoptCurrent')}
                        </Button>
                      ) : null}
                    </div>
                  </div>

                  {run.detected_variables.length > 0 ? (
                    <div className="flex flex-wrap gap-2">
                      {run.detected_variables.map(variable => (
                        <Badge key={variable} variant="outline">
                          {variable}
                        </Badge>
                      ))}
                    </div>
                  ) : null}

                  <Textarea
                    value={run.output}
                    readOnly
                    className="min-h-[220px] font-mono text-xs"
                  />
                </div>
              );
            })}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

export default PromptOptimizationHistory;
