'use client';

import { useId } from 'react';
import { AlertTriangle, Eye, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
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
import type { AgentBindingHealth } from '@/services/types/agent';

interface AgentRuntimePreviewPanelProps {
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
  bindingHealth?: AgentBindingHealth;
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
  openingGuideBrand,
  homeTitle,
  openingStatement,
  bindingHealth,
  surfaceMode = 'inline',
  onOpenMemoryValues,
  onModelChange,
  beforeSend,
  onClose,
}: AgentRuntimePreviewPanelProps) {
  const t = useT('agents.agentRuntime');
  const controlsPortalId = useId();
  const ignoredBindings = bindingHealth?.items.filter(item => item.status !== 'active') ?? [];

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
      {ignoredBindings.length > 0 ? (
        <div className="shrink-0 px-4 pb-2">
          <Alert className="py-2">
            <AlertTriangle className="size-4" />
            <AlertTitle>{t('bindingHealth.previewIgnoredTitle')}</AlertTitle>
            <AlertDescription className="line-clamp-2 text-xs">
              {t('bindingHealth.previewIgnoredDescription', {
                count: ignoredBindings.length,
                resources: ignoredBindings
                  .map(item => item.display_name || item.resource_id)
                  .join(', '),
              })}
            </AlertDescription>
          </Alert>
        </div>
      ) : null}
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
        />
      </div>
    </section>
  );
}
