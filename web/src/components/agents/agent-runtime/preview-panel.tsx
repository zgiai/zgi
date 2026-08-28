'use client';

import { useCallback, useId } from 'react';
import { Eye, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { AIChatModelPrecheckWarning } from '@/components/aichat/model-precheck-warning';
import Chat, { type AIChatController } from '@/components/chat';
import {
  AIChatEmbeddedConversationControls,
  embeddedControlButtonClassName,
} from '@/components/chat/variants/aichat/embedded-conversation-controls';
import type {
  ModelSelectorModelProps,
  ModelSelectorParameterValue,
  ModelSelectorValue,
} from '@/components/common/model-selector';
import { useT } from '@/i18n';
import type { OpeningGuideBrand } from '@/components/chat/utils/opening-guide-brand';
import { useAgentDraftModelPrecheck } from '@/hooks/agent/use-agent-draft-model-precheck';
import { transcribeAgentDraftVoice } from '@/services/voice-transcription.service';
import { generateAgentDraftSpeech } from '@/services/voice-speech.service';
import {
  allowSendAfterAgentModelPrecheck,
  visibleAgentModelPrecheckWarnings,
} from './model-precheck';

interface AgentRuntimePreviewPanelProps {
  agentId: string;
  controller: AIChatController;
  modelSelectorValue: ModelSelectorParameterValue;
  modelProps?: ModelSelectorModelProps | null;
  useMemory: boolean;
  fileUploadEnabled: boolean;
  suggestions: string[];
  inputPlaceholder: string;
  openingGuideBrand: OpeningGuideBrand;
  homeTitle: string;
  openingStatement: string;
  voiceInputEnabled: boolean;
  speechEnabled: boolean;
  surfaceMode?: 'inline' | 'sheet';
  onOpenMemoryValues: () => void;
  onModelChange: (value: ModelSelectorValue) => void;
  beforeSend?: () => boolean | Promise<boolean>;
  onClose?: () => void;
}

export function AgentRuntimePreviewPanel({
  agentId,
  controller,
  modelSelectorValue,
  modelProps,
  useMemory,
  fileUploadEnabled,
  suggestions,
  inputPlaceholder,
  openingGuideBrand,
  homeTitle,
  openingStatement,
  voiceInputEnabled,
  speechEnabled,
  surfaceMode = 'inline',
  onOpenMemoryValues,
  onModelChange,
  beforeSend,
  onClose,
}: AgentRuntimePreviewPanelProps) {
  const t = useT('agents.agentRuntime');
  const controlsPortalId = useId();
  const modelPrecheck = useAgentDraftModelPrecheck(
    agentId,
    modelSelectorValue.provider,
    modelSelectorValue.model
  );
  const warnings = visibleAgentModelPrecheckWarnings(modelPrecheck.data);
  const handleVoiceTranscription = useCallback(
    (audio: ArrayBuffer, signal: AbortSignal) => transcribeAgentDraftVoice(agentId, audio, signal),
    [agentId]
  );
  const handleSpeechSynthesis = useCallback(
    (input: string, signal: AbortSignal) => generateAgentDraftSpeech(agentId, input, signal),
    [agentId]
  );
  const handleBeforeSend = async () => {
    if (beforeSend && !(await beforeSend())) {
      return false;
    }
    return allowSendAfterAgentModelPrecheck(modelPrecheck.refetch);
  };

  return (
    <section className="flex h-full min-h-0 w-full min-w-0 flex-col overflow-hidden">
      <div className="flex h-14 shrink-0 items-center justify-between gap-2 px-5">
        <div className="min-w-[3rem] shrink-0">
          <h2 className="whitespace-nowrap text-sm font-semibold">{t('preview.title')}</h2>
          {t('preview.description') ? (
            <p className="truncate text-xs text-muted-foreground">{t('preview.description')}</p>
          ) : null}
        </div>
        <div className="flex min-w-0 shrink items-center justify-end">
          <div className="flex items-center gap-1">
            <Button
              type="button"
              variant="ghost"
              isIcon
              interactive="subtle"
              className={embeddedControlButtonClassName}
              onClick={onOpenMemoryValues}
              aria-label={t('memory.viewValues')}
              title={t('memory.viewValues')}
            >
              <Eye className="size-3.5" />
            </Button>
            <div id={controlsPortalId} className="flex shrink-0 items-center" />
            {surfaceMode === 'sheet' ? (
              <Button
                type="button"
                variant="ghost"
                isIcon
                interactive="subtle"
                className={embeddedControlButtonClassName}
                aria-label={t('preview.close')}
                title={t('preview.close')}
                onClick={onClose}
              >
                <X className="size-3.5" />
              </Button>
            ) : null}
          </div>
        </div>
      </div>
      <div className="min-h-0 flex-1">
        <Chat
          mode="aichat"
          controller={controller}
          modelSelectorValue={modelSelectorValue}
          modelProps={modelProps}
          onModelChange={onModelChange}
          beforeSend={handleBeforeSend}
          inputTopNotice={
            warnings.length > 0 ? <AIChatModelPrecheckWarning warnings={warnings} /> : undefined
          }
          variant="embedded"
          showModelSelector={false}
          showMemoryToggle={false}
          forcedUseMemory={useMemory}
          enableUpload={fileUploadEnabled}
          suggestions={suggestions}
          inputPlaceholder={inputPlaceholder}
          embeddedConversationMode="drawer"
          embeddedConversationControlsMode="external"
          embeddedConversationControlsPortalId={controlsPortalId}
          renderEmbeddedConversationControls={controls => (
            <AIChatEmbeddedConversationControls
              openConversations={controls.openConversations}
              startNewConversation={controls.startNewConversation}
              conversationsLabel={t('preview.conversations')}
              newConversationLabel={t('preview.newConversation')}
            />
          )}
          showAssistantModelMeta={false}
          surface="agent-draft"
          openingGuideBrand={openingGuideBrand}
          homeTitle={homeTitle}
          homeDescription={openingStatement}
          voiceTranscriber={voiceInputEnabled ? handleVoiceTranscription : undefined}
          speechSynthesizer={speechEnabled ? handleSpeechSynthesis : undefined}
        />
      </div>
    </section>
  );
}
