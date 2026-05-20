'use client';

// Data ingestion page under a table – sibling to "create" page.
// Step 1: select files; Step 2: placeholder for AI recognition & preview.

import { use } from 'react';
import { useState } from 'react';
import Link from 'next/link';
import { Button } from '@/components/ui/button';
import { ChevronLeft, ArrowLeft, RefreshCcw, Sparkles } from 'lucide-react';
import { TableIngestProgressBar } from '@/components/db/table-ingest/table-ingest-progress-bar';
import { StepOne, StepTwo } from '@/components/db/table-ingest';
import TablePromptDialog from '@/components/db/table-ingest/table-prompt-dialog';
import { useT } from '@/i18n';
import type { FileItem } from '@/services/types/file';
import { ModelSelector } from '@/components/common/model-selector';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import { useInitializeDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useCurrentUser } from '@/store/auth-store';
import { getLastSelectedAiModel, saveLastSelectedAiModel } from '@/utils/ui-local';

interface PageProps {
  params: Promise<{ dbId: string; tableId: string }>;
}

type Step = 1 | 2;

export default function DbTableDataIngestPage({ params }: PageProps) {
  const { dbId, tableId } = use(params);
  const t = useT();

  const [step, setStep] = useState<Step>(1);
  const [selectedFiles, setSelectedFiles] = useState<FileItem[]>([]);
  const [selectedModel, setSelectedModel] = useState<{ provider: string; model: string } | null>(
    null
  );
  const [promptOpen, setPromptOpen] = useState<boolean>(false);
  // Nonce used to trigger re-recognition in StepTwo
  const [reRecognitionNonce, setReRecognitionNonce] = useState<number>(0);

  // Initialize model selection from saved preference or workspace default
  const user = useCurrentUser();
  useInitializeDefaultModelByUseCase({
    useCase: 'text-chat',
    currentModel: selectedModel ?? {},
    enabled: Boolean(user?.id && !getLastSelectedAiModel(user.id, 'ingest')),
    onInitialize: v => {
      setSelectedModel({ provider: v.provider, model: v.model });
    },
  });

  const onPrevStep = () => setStep(1);

  return (
    <div className="p-6 h-full flex flex-col w-full overflow-hidden">
      {/* Page-specific progress bar for data ingestion */}
      <div className="mb-4">
        <TableIngestProgressBar
          currentStep={step === 1 ? 1 : 3}
          totalSteps={4}
          allowStepNavigation
          onStepClick={s => {
            if (s === 1) setStep(1);
            if (s > 1 && selectedFiles.length > 0) setStep(2);
          }}
        />
      </div>

      {/* Header with back button */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-1">
          <Link
            href={`/console/db/${dbId}/table/${tableId}`}
            className="hover:bg-muted flex justify-center items-center w-9 h-9 rounded-md"
          >
            <ArrowLeft className="h-5 w-5" />
          </Link>
          <span className="font-semibold text-lg">{t('dbs.dataIngestPage.headerTitle')}</span>
        </div>
        <div className="flex items-center gap-2">
          {step === 2 && (
            <Button variant="outline" onClick={onPrevStep} className="gap-1">
              <ChevronLeft className="h-4 w-4" /> {t('common.previous')}
            </Button>
          )}
          {/* LLM model selector for ingest – required. Placed next to prompt button. */}
          {(step === 1 || step === 2) && (
            <div className="w-64">
              <ModelSelector
                modelType="text-chat"
                value={selectedModel ?? undefined}
                onChange={(value: ModelSelectorValue) => {
                  setSelectedModel(value);
                  if (user?.id) {
                    saveLastSelectedAiModel(user.id, 'ingest', {
                      provider: value.provider,
                      model: value.model,
                    });
                  }
                }}
                placeholder={t('dbs.modelSelector.placeholder', {
                  defaultMessage: 'Select a model',
                })}
              />
            </div>
          )}
          <Button
            className="bg-highlight hover:bg-highlight/90"
            onClick={() => setPromptOpen(true)}
          >
            <Sparkles className="h-4 w-4" />
            {t('dbs.promptDialog.title')}
          </Button>
          {step === 2 && (
            <Button
              onClick={() => setReRecognitionNonce(n => n + 1)}
              disabled={selectedFiles.length === 0 || !selectedModel}
            >
              <RefreshCcw className="h-4 w-4" />
              {t('dbs.tableIngest.stepTwo.reRecognize')}
            </Button>
          )}
        </div>
      </div>

      {step === 1 && (
        <StepOne
          onNext={files => {
            setSelectedFiles(files);
            setStep(2);
          }}
          modelSelected={Boolean(selectedModel)}
          initialFiles={selectedFiles}
        />
      )}

      {step === 2 && selectedModel && (
        <StepTwo
          selectedFiles={selectedFiles}
          selectedModel={selectedModel}
          dbId={dbId}
          tableId={tableId}
          reRecognitionNonce={reRecognitionNonce}
        />
      )}

      {/* Prompt management dialog */}
      <TablePromptDialog
        open={promptOpen}
        onOpenChange={setPromptOpen}
        dbId={dbId}
        tableId={tableId}
      />
    </div>
  );
}
