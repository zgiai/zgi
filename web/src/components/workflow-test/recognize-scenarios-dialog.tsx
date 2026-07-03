'use client';

import * as React from 'react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { ModelSelector } from '@/components/common/model-selector';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import { useCreateWorkflowTestScenarioRecognitionTask } from '@/hooks/workflow-test/use-workflow-test';
import { useDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useCurrentUser } from '@/store/auth-store';
import { getLastSelectedAiModel, saveLastSelectedAiModel } from '@/utils/ui-local';
import { useT } from '@/i18n';

interface RecognizeScenariosDialogProps {
  agentId: string;
  defaultContext: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function RecognizeScenariosDialog({
  agentId,
  defaultContext,
  open,
  onOpenChange,
}: RecognizeScenariosDialogProps) {
  const t = useT('agents.workflowTest.dialogs.recognizeScenarios');
  const commonT = useT('agents.workflowTest.common');
  const createRecognitionTask = useCreateWorkflowTestScenarioRecognitionTask(agentId);
  const user = useCurrentUser();
  const { value: defaultModel } = useDefaultModelByUseCase('text-chat');
  const [selectedModel, setSelectedModel] = React.useState<ModelSelectorValue | null>(() => {
    if (!user?.id) return null;
    const saved = getLastSelectedAiModel(user.id, 'workflowTestScenario');
    return saved ? { provider: saved.provider, model: saved.model } : null;
  });
  const [prompt, setPrompt] = React.useState('');

  React.useEffect(() => {
    if (defaultModel && !selectedModel && user?.id) {
      const saved = getLastSelectedAiModel(user.id, 'workflowTestScenario');
      if (!saved) {
        setSelectedModel({ provider: defaultModel.provider, model: defaultModel.model });
      }
    }
  }, [defaultModel, selectedModel, user?.id]);

  React.useEffect(() => {
    if (open) {
      setPrompt(t('promptDefault'));
    }
  }, [open, t]);

  const canSubmit = Boolean(selectedModel?.provider && selectedModel?.model && prompt.trim());

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        size="lg"
        className="max-w-[760px] rounded-2xl"
        onInteractOutside={event => event.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>{t('title')}</DialogTitle>
          <DialogDescription>{t('description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-5">
          <div className="space-y-2">
            <Label>{t('modelLabel')}</Label>
            <ModelSelector
              modelType="text-chat"
              value={selectedModel ?? undefined}
              onChange={value => {
                setSelectedModel(value);
                if (user?.id) {
                  saveLastSelectedAiModel(user.id, 'workflowTestScenario', {
                    provider: value.provider,
                    model: value.model,
                  });
                }
              }}
              placeholder={t('modelPlaceholder')}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="workflow-test-recognize-prompt">{t('promptLabel')}</Label>
            <Textarea
              id="workflow-test-recognize-prompt"
              value={prompt}
              onChange={event => setPrompt(event.target.value)}
              placeholder={t('promptPlaceholder')}
              className="min-h-56 resize-none leading-7"
            />
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {commonT('cancel')}
          </Button>
          <Button
            disabled={createRecognitionTask.isPending || !canSubmit}
            onClick={() => {
              if (!selectedModel) return;
              createRecognitionTask.mutate(
                {
                  context: defaultContext,
                  prompt,
                  model: {
                    provider: selectedModel.provider,
                    name: selectedModel.model,
                  },
                },
                { onSuccess: () => onOpenChange(false) }
              );
            }}
          >
            {createRecognitionTask.isPending ? t('submitting') : t('submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
