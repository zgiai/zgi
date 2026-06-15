'use client';

import { useId, type ReactNode } from 'react';
import { Eye, MessageSquarePlus, PanelLeft, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import Chat, { type AIChatController } from '@/components/chat';
import type {
  ModelSelectorModelProps,
  ModelSelectorParameterValue,
  ModelSelectorValue,
} from '@/components/common/model-selector';
import { useT } from '@/i18n';

interface AgentRuntimePreviewPanelProps {
  controller: AIChatController;
  modelSelectorValue: ModelSelectorParameterValue;
  modelProps?: ModelSelectorModelProps | null;
  useMemory: boolean;
  fileUploadEnabled: boolean;
  suggestions: string[];
  inputPlaceholder: string;
  homeBrand: ReactNode;
  homeTitle: string;
  surfaceMode?: 'inline' | 'sheet';
  onOpenMemoryValues: () => void;
  onModelChange: (value: ModelSelectorValue) => void;
  beforeSend?: () => boolean | Promise<boolean>;
  onClose?: () => void;
}

export function AgentRuntimePreviewPanel({
  controller,
  modelSelectorValue,
  modelProps,
  useMemory,
  fileUploadEnabled,
  suggestions,
  inputPlaceholder,
  homeBrand,
  homeTitle,
  surfaceMode = 'inline',
  onOpenMemoryValues,
  onModelChange,
  beforeSend,
  onClose,
}: AgentRuntimePreviewPanelProps) {
  const t = useT('agents.agentRuntime');
  const controlsPortalId = useId();

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
          <div className="flex items-center gap-1 rounded-full border bg-background p-1 shadow-sm">
            <Button
              variant="ghost"
              size="sm"
              interactive="subtle"
              className="h-7 min-w-0 rounded-full px-2 text-xs hover:bg-muted/70"
              onClick={onOpenMemoryValues}
              title={t('memory.viewValues')}
            >
              <Eye className="size-3.5" />
              <span className="hidden sm:inline">{t('memory.viewValues')}</span>
            </Button>
            <div id={controlsPortalId} className="flex shrink-0 items-center" />
            {surfaceMode === 'sheet' ? (
              <Button
                variant="ghost"
                isIcon
                size="sm"
                interactive="subtle"
                className="size-7 rounded-full hover:bg-muted/70"
                aria-label={t('preview.close')}
                title={t('preview.close')}
                onClick={onClose}
              >
                <X className="size-[18px]" />
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
          beforeSend={beforeSend}
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
            <div className="flex items-center gap-1">
              <Button
                variant="ghost"
                isIcon
                className="size-7 rounded-full text-muted-foreground hover:bg-muted/70 hover:text-foreground"
                onClick={controls.openConversations}
                title={t('preview.conversations')}
                aria-label={t('preview.conversations')}
              >
                <PanelLeft className="size-3.5" />
              </Button>
              <Button
                variant="ghost"
                isIcon
                className="size-7 rounded-full text-muted-foreground hover:bg-muted/70 hover:text-foreground"
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
