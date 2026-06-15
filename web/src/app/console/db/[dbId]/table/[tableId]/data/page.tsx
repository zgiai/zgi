'use client';

// Data ingestion page under a table – sibling to "create" page.
// Step 1: select files; Step 2: placeholder for AI recognition & preview.

import { use, useCallback, useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { Button } from '@/components/ui/button';
import { ChevronLeft, ArrowLeft, Sparkles } from 'lucide-react';
import { StepOne, StepTwo } from '@/components/db/table-ingest';
import TablePromptDialog from '@/components/db/table-ingest/table-prompt-dialog';
import { useT } from '@/i18n';
import type { FileItem } from '@/services/types/file';
import { ModelSelector } from '@/components/common/model-selector';
import type {
  ModelSelectorModelProps,
  ModelSelectorValue,
} from '@/components/common/model-selector';
import { useInitializeDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useCurrentUser } from '@/store/auth-store';
import { getLastSelectedAiModel, saveLastSelectedAiModel } from '@/utils/ui-local';
import { TABLE_INGEST_ALL_EXTENSIONS } from '@/components/db/table-ingest/file-support';
import { useTableIngestLeaveGuard } from '@/components/db/table-ingest/use-table-ingest-leave-guard';

interface PageProps {
  params: Promise<{ dbId: string; tableId: string }>;
}

type Step = 1 | 2;

export default function DbTableDataIngestPage({ params }: PageProps) {
  const { dbId, tableId } = use(params);
  const t = useT();
  const router = useRouter();

  const [step, setStep] = useState<Step>(1);
  const [selectedFiles, setSelectedFiles] = useState<FileItem[]>([]);
  const [selectedModel, setSelectedModel] = useState<{ provider: string; model: string } | null>(
    null
  );
  const [selectedModelProps, setSelectedModelProps] = useState<ModelSelectorModelProps | null>(
    null
  );
  const [modelPreferenceChecked, setModelPreferenceChecked] = useState(false);
  const [promptOpen, setPromptOpen] = useState<boolean>(false);
  const [leaveGuard, setLeaveGuard] = useState<{
    active: boolean;
    reason: 'processing' | 'unsaved' | null;
  }>({ active: false, reason: null });

  // Initialize model selection from saved preference or workspace default
  const user = useCurrentUser();
  useEffect(() => {
    if (!user?.id || modelPreferenceChecked) return;

    const savedModel = getLastSelectedAiModel(user.id, 'ingest');
    if (savedModel) {
      setSelectedModel(savedModel);
    }
    setModelPreferenceChecked(true);
  }, [modelPreferenceChecked, user?.id]);

  useInitializeDefaultModelByUseCase({
    useCase: 'text-chat',
    currentModel: selectedModel ?? {},
    enabled: modelPreferenceChecked && !selectedModel,
    onInitialize: v => {
      setSelectedModel({ provider: v.provider, model: v.model });
    },
  });

  const handleLeaveGuardChange = useCallback(
    (active: boolean, reason: 'processing' | 'unsaved' | null) => {
      setLeaveGuard(prev => {
        if (prev.active === active && prev.reason === reason) return prev;
        return { active, reason };
      });
    },
    []
  );
  const { leaveGuardDialog, confirmNavigation } = useTableIngestLeaveGuard({
    enabled: leaveGuard.active,
    reason: leaveGuard.reason,
  });

  const onPrevStep = () => {
    confirmNavigation(() => setStep(1));
  };
  const selectableExtensions = [...TABLE_INGEST_ALL_EXTENSIONS];
  const handleModelPropsChange = useCallback((props: ModelSelectorModelProps | null) => {
    setSelectedModelProps(props);
  }, []);
  const handleModelChange = useCallback(
    (value: ModelSelectorValue) => {
      setSelectedModel(value);
      setSelectedModelProps(null);
      if (user?.id) {
        saveLastSelectedAiModel(user.id, 'ingest', {
          provider: value.provider,
          model: value.model,
        });
      }
    },
    [user?.id]
  );

  return (
    <div className="p-4 h-full flex flex-col w-full overflow-hidden">
      {/* Header with back button */}
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={() => {
              confirmNavigation(() => router.push(`/console/db/${dbId}/table/${tableId}`));
            }}
            className="hover:bg-muted flex justify-center items-center w-9 h-9 rounded-md"
          >
            <ArrowLeft className="h-5 w-5" />
          </button>
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
            <div className="flex items-center gap-1">
              <div className="w-64">
                <ModelSelector
                  modelType="text-chat"
                  value={selectedModel ?? undefined}
                  modelProps={selectedModelProps}
                  onModelPropsChange={handleModelPropsChange}
                  onChange={handleModelChange}
                  placeholder={t('dbs.modelSelector.placeholder', {
                    defaultMessage: 'Select a model',
                  })}
                />
              </div>
            </div>
          )}
          <Button
            className="bg-highlight hover:bg-highlight/90"
            onClick={() => setPromptOpen(true)}
          >
            <Sparkles className="h-4 w-4" />
            {t('dbs.promptDialog.title')}
          </Button>
        </div>
      </div>

      {step === 1 && (
        <StepOne
          onNext={files => {
            setSelectedFiles(files);
            setStep(2);
          }}
          onFilesChange={setSelectedFiles}
          modelSelected={Boolean(selectedModel)}
          initialFiles={selectedFiles}
          acceptExt={selectableExtensions}
        />
      )}

      {step === 2 && selectedModel && (
        <StepTwo
          selectedFiles={selectedFiles}
          selectedModel={selectedModel}
          dbId={dbId}
          tableId={tableId}
          onRemoveFile={fileId => {
            setSelectedFiles(prev => prev.filter(file => file.id !== fileId));
          }}
          onLeaveGuardChange={handleLeaveGuardChange}
        />
      )}

      {/* Prompt management dialog */}
      <TablePromptDialog
        open={promptOpen}
        onOpenChange={setPromptOpen}
        dbId={dbId}
        tableId={tableId}
      />
      {leaveGuardDialog}
    </div>
  );
}
