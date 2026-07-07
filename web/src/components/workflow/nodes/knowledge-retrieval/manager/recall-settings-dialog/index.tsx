'use client';

import React, { useCallback, useMemo, useState } from 'react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Slider } from '@/components/ui/slider';
import { Switch } from '@/components/ui/switch';
import { ModelSelector } from '@/components/common/model-selector';
import { useT } from '@/i18n';
import { useNodeData, useNodeDataUpdate } from '../../../../hooks';
import type { KnowledgeRetrievalNodeData } from '../../../../store/type';

interface RecallSettingsDialogProps {
  id: string;
  readOnly?: boolean;
}

const DEFAULT_SCORE_THRESHOLD = 0.8;

const RecallSettingsDialog: React.FC<RecallSettingsDialogProps> = ({ id, readOnly = false }) => {
  const [open, setOpen] = useState(false);
  const t = useT();
  const nodeData = useNodeData<KnowledgeRetrievalNodeData>(id);
  const updateNodeData = useNodeDataUpdate<KnowledgeRetrievalNodeData>(id);

  const config = useMemo(
    () =>
      nodeData?.multiple_retrieval_config || {
        top_k: 2,
        reranking_enable: false,
        score_threshold: DEFAULT_SCORE_THRESHOLD,
      },
    [nodeData?.multiple_retrieval_config]
  );

  const topKSafe = Math.max(1, Number(config.top_k ?? 1));
  const thresholdEnabled = typeof config.score_threshold === 'number';
  const thresholdSafe = Math.max(0, Math.min(1, Number(config.score_threshold ?? 0)));
  const rerankingEnabled = Boolean(config.reranking_enable);
  const rerankingModelValue =
    config.reranking_model?.provider && config.reranking_model?.model
      ? {
          provider: config.reranking_model.provider,
          model: config.reranking_model.model,
        }
      : undefined;

  const update = useCallback(
    (patch: Partial<KnowledgeRetrievalNodeData['multiple_retrieval_config']>) => {
      if (readOnly || !nodeData) return;
      updateNodeData({
        multiple_retrieval_config: {
          ...config,
          ...patch,
        } as KnowledgeRetrievalNodeData['multiple_retrieval_config'],
      });
    },
    [config, nodeData, readOnly, updateNodeData]
  );

  if (!nodeData) return null;

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm" variant="outline">
          {t('nodes.knowledgeRetrieval.recall.trigger')}
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-[640px] p-0 overflow-hidden">
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-bold tracking-tight">
            {t('nodes.knowledgeRetrieval.recall.title')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="py-6">
          <div className="space-y-6">
            <div className="space-y-3">
              <Label htmlFor="knowledge-top-k" className="text-sm font-medium">
                {t('nodes.knowledgeRetrieval.recall.topK')}
              </Label>
              <div className="flex items-center gap-6">
                <Slider
                  min={1}
                  max={10}
                  step={1}
                  value={[topKSafe]}
                  onValueChange={([value]) => update({ top_k: Math.max(1, Number(value)) })}
                  disabled={readOnly}
                  className="flex-1"
                />
                <Input
                  id="knowledge-top-k"
                  type="number"
                  min={1}
                  max={10}
                  step={1}
                  className="h-9 w-20 text-center"
                  value={topKSafe}
                  disabled={readOnly}
                  onChange={event =>
                    update({ top_k: Math.max(1, Number(event.target.value || 1)) })
                  }
                />
              </div>
            </div>

            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <Label htmlFor="knowledge-score-threshold" className="text-sm font-medium">
                  {t('nodes.knowledgeRetrieval.recall.scoreThreshold')}
                </Label>
                <Switch
                  checked={thresholdEnabled}
                  disabled={readOnly}
                  onCheckedChange={checked =>
                    update({
                      score_threshold: checked
                        ? (config.score_threshold ?? DEFAULT_SCORE_THRESHOLD)
                        : undefined,
                    })
                  }
                />
              </div>
              <div className="flex items-center gap-6">
                <Slider
                  min={0}
                  max={1}
                  step={0.05}
                  value={[thresholdSafe]}
                  onValueChange={([value]) => update({ score_threshold: Number(value) })}
                  disabled={readOnly || !thresholdEnabled}
                  className="flex-1"
                />
                <Input
                  id="knowledge-score-threshold"
                  type="number"
                  min={0}
                  max={1}
                  step={0.05}
                  className="h-9 w-20 text-center font-mono"
                  value={thresholdSafe}
                  disabled={readOnly || !thresholdEnabled}
                  onChange={event => {
                    const nextValue = Math.max(
                      0,
                      Math.min(1, Number.parseFloat(event.target.value || '0'))
                    );
                    update({ score_threshold: nextValue });
                  }}
                />
              </div>
            </div>

            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <Label className="text-sm font-medium">
                  {t('nodes.knowledgeRetrieval.recall.enableReranking')}
                </Label>
                <Switch
                  checked={rerankingEnabled}
                  disabled={readOnly}
                  onCheckedChange={checked =>
                    update({
                      reranking_enable: checked,
                      reranking_mode: checked ? 'reranking_model' : config.reranking_mode,
                    })
                  }
                />
              </div>
              {rerankingEnabled ? (
                <ModelSelector
                  modelType="rerank"
                  value={rerankingModelValue}
                  onChange={({ provider, model }) =>
                    update({
                      reranking_mode: 'reranking_model',
                      reranking_model: {
                        provider,
                        model,
                      },
                    })
                  }
                  className="max-w-md"
                  disabled={readOnly}
                />
              ) : null}
            </div>
          </div>
        </DialogBody>

        <DialogFooter className="border-t bg-neutral-50/50 px-6 pb-6 pt-4">
          <Button variant="ghost" onClick={() => setOpen(false)} className="font-semibold">
            {t('common.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default RecallSettingsDialog;
