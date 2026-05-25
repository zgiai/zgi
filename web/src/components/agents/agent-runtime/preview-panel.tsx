'use client';

import { useId, type ReactNode } from 'react';
import { MessageSquarePlus, PanelLeft } from 'lucide-react';
import { Button } from '@/components/ui/button';
import Chat, { type AIChatController } from '@/components/chat';
import type { ModelSelectorParameterValue, ModelSelectorValue } from '@/components/common/model-selector';
import { useT } from '@/i18n';

interface AgentRuntimePreviewPanelProps {
  controller: AIChatController;
  modelSelectorValue: ModelSelectorParameterValue;
  useMemory: boolean;
  fileUploadEnabled: boolean;
  suggestions: string[];
  inputPlaceholder: string;
  homeBrand: ReactNode;
  homeTitle: string;
  onModelChange: (value: ModelSelectorValue) => void;
}

export function AgentRuntimePreviewPanel({
  controller,
  modelSelectorValue,
  useMemory,
  fileUploadEnabled,
  suggestions,
  inputPlaceholder,
  homeBrand,
  homeTitle,
  onModelChange,
}: AgentRuntimePreviewPanelProps) {
  const t = useT('agents.agentRuntime');
  const controlsPortalId = useId();

  return (
    <section className="flex min-w-0 flex-col overflow-hidden">
      <div className="flex h-12 shrink-0 items-center justify-between px-5">
        <div>
          <h2 className="text-sm font-semibold">{t('preview.title')}</h2>
          <p className="text-xs text-muted-foreground">{t('preview.description')}</p>
        </div>
        <div className="flex items-center gap-2">
          <div id={controlsPortalId} className="flex shrink-0 items-center" />
        </div>
      </div>
      <div className="min-h-0 flex-1">
        <Chat
          mode="aichat"
          controller={controller}
          modelSelectorValue={modelSelectorValue}
          onModelChange={onModelChange}
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
            <div className="flex items-center gap-1 rounded-full border bg-background p-1 shadow-sm">
              <Button
                variant="ghost"
                isIcon
                className="size-7 text-muted-foreground"
                onClick={controls.openConversations}
                title={t('preview.conversations')}
                aria-label={t('preview.conversations')}
              >
                <PanelLeft className="size-3.5" />
              </Button>
              <Button
                variant="ghost"
                isIcon
                className="size-7 text-muted-foreground"
                onClick={controls.startNewConversation}
                title={t('preview.newConversation')}
                aria-label={t('preview.newConversation')}
              >
                <MessageSquarePlus className="size-3.5" />
              </Button>
            </div>
          )}
          showAssistantModelMeta={false}
          surface="agent-draft"
          homeBrand={homeBrand}
          homeTitle={homeTitle}
          homeDescription=""
        />
      </div>
    </section>
  );
}
