'use client';

import { useEffect, useId, useMemo } from 'react';
import { History, Info, X } from 'lucide-react';
import { AIChatShell, createDigitalHumanRuntimeTransport, useChatRuntimeController } from '@/components/chat';
import {
  AIChatEmbeddedConversationControls,
  embeddedControlButtonClassName,
} from '@/components/chat/variants/aichat/embedded-conversation-controls';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import type { FileUploadSelection } from '@/services/upload.service';

/**
 * Chats with the unpublished draft beside the configuration, like an Agent's
 * preview. Preview runs keep their own conversations and never create tasks,
 * save artifacts, change memory, or act in connected systems.
 */
export function DigitalHumanDraftPreview({
  id,
  name,
  model,
  fileUploadPolicy,
  surfaceMode,
  pausedVersion,
  onClose,
  beforeSend,
}: {
  id: string;
  name: string;
  model: string;
  fileUploadPolicy: FileUploadSelection | null;
  surfaceMode: 'inline' | 'sheet';
  // Set while a published version is shown: the preview pauses over its chat.
  pausedVersion?: string | number;
  onClose?: () => void;
  // Saves the draft first, so the preview always answers with what is on screen.
  beforeSend: () => boolean | Promise<boolean>;
}) {
  const t = useT('digitalHumans');
  const controlsPortalId = useId();
  const transport = useMemo(() => createDigitalHumanRuntimeTransport(id, { preview: true }), [id]);
  const controller = useChatRuntimeController({ transport, requireModel: false, memoryMutationMode: 'disabled' });
  const initController = controller.init;
  useEffect(() => {
    initController(null);
  }, [initController]);
  const modelSelectorValue = useMemo(() => ({ provider: '', model, params: {} }), [model]);

  return (
    <section className="flex h-full min-h-0 w-full min-w-0 flex-col overflow-hidden">
      <div className="flex h-14 shrink-0 items-center justify-between gap-2 border-b px-4">
        <div className="min-w-0">
          <h2 className="text-sm font-semibold">{t('preview.title')}</h2>
          <p className="truncate text-xs text-muted-foreground">{t('preview.description')}</p>
        </div>
        <div className="flex shrink-0 items-center gap-1">
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
      <p className="flex shrink-0 items-start gap-1.5 border-b bg-muted/30 px-4 py-2 text-xs leading-5 text-muted-foreground">
        <Info className="mt-0.5 size-3.5 shrink-0" />
        {t('preview.sandboxNote')}
      </p>
      <div className="relative min-h-0 flex-1">
        {pausedVersion !== undefined ? (
          <div className="absolute inset-0 z-30 flex items-center justify-center bg-background p-6">
            <div className="flex max-w-xs flex-col items-center gap-2 text-center">
              <span className="flex size-9 items-center justify-center rounded-full bg-muted text-muted-foreground">
                <History className="size-4" />
              </span>
              <p className="text-sm font-medium">{t('preview.pausedTitle', { version: pausedVersion })}</p>
              <p className="text-xs leading-5 text-muted-foreground">{t('preview.pausedDescription')}</p>
            </div>
          </div>
        ) : null}
        {/* Hidden, not unmounted, so the preview chat is still there afterwards. */}
        <div className={pausedVersion !== undefined ? 'invisible h-full' : 'h-full'} aria-hidden={pausedVersion !== undefined}>
          <AIChatShell
            controller={controller}
            modelSelectorValue={modelSelectorValue}
            onModelChange={() => undefined}
            beforeSend={beforeSend}
            showModelSelector={false}
            variant="embedded"
            requireModel={false}
            forcedUseMemory={false}
            enableUpload={fileUploadPolicy == null || fileUploadPolicy.allowed_file_types.length > 0}
            fileUploadPolicy={fileUploadPolicy}
            uploadScope={{ type: 'console' }}
            showFileLibraryPicker={false}
            allowWorkspaceSwitch={false}
            showAssistantModelMeta={false}
            surface="digital-human"
            runtimeSurface="external_page_chat"
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
            homeTitle={t('preview.homeTitle', { name: name || t('title') })}
            homeDescription={t('preview.homeDescription')}
            suggestions={[t('chat.suggestions.introduce'), t('chat.suggestions.capabilities'), t('chat.suggestions.plan')]}
            inputPlaceholder={t('chat.placeholder')}
          />
        </div>
      </div>
    </section>
  );
}
