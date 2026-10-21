'use client';

import * as React from 'react';
import { useChatStore } from '@/components/chat/store';
import { InputArea, type ImageReferenceAttachment } from './input-area';
import { ImageHomeView } from './home-view';
import { Sidebar } from '../common/sidebar';
import { PanelLeft, Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import type { ChatController } from '@/components/chat/controllers/types';
import { cn } from '@/lib/utils';
import { useSysImageStore } from './store';
import { useStore } from 'zustand';
import { useT } from '@/i18n/translations';
import { toast } from 'sonner';
import { SysChatSkeleton } from '../sys/skeleton';
import type { Message as ChatTypeMessage } from '@/components/chat/types';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import { useIsMobile } from '@/hooks/use-mobile';
import { Sheet, SheetContent } from '@/components/ui/sheet';
import { useChatAutoFollow } from '../common/use-chat-auto-follow';
import { ChatMessageViewport } from '../common/chat-message-viewport';
import type { ImageSettings, ImageSettingsPatch } from './settings-toolbar';
import type { ImageRuntimeModel } from '@/services/types/image-runtime';
import { getImagePromptCharacterCount, IMAGE_PROMPT_MAX_CHARACTERS } from './constants';

export interface ImgChatProps {
  controller: ChatController;
  modelSelectorValue?: ModelSelectorValue;
  onModelChange?: (value: ModelSelectorValue) => void;
  inputTopNotice?: React.ReactNode;
  conversationSearchKey?: readonly unknown[];
  imageRuntimeModels?: ImageRuntimeModel[];
}

export function ImgChat({
  controller,
  modelSelectorValue,
  onModelChange,
  inputTopNotice,
  conversationSearchKey,
  imageRuntimeModels,
}: ImgChatProps) {
  // Controller state
  const activeId = useStore(controller.store, s => s.activeId);
  const conversationList = useStore(controller.store, s => s.conversations);
  const isSending = useStore(controller.store, s => s.isSending);
  const isLoadingDetail = useStore(controller.store, s => s.isLoadingDetail);

  const t = useT('webapp');

  // Store state for messages
  const currentConversation = useChatStore(state =>
    activeId ? state.conversations[activeId] : undefined
  );

  const messages: ChatTypeMessage[] = React.useMemo(() => {
    if (!currentConversation) return [];
    return currentConversation.messages;
  }, [currentConversation]);

  const [input, setInput] = React.useState('');
  const [referenceImage, setReferenceImage] = React.useState<ImageReferenceAttachment | null>(
    null
  );
  const [isSidebarOpen, setIsSidebarOpen] = React.useState(true);
  const [isMobileSidebarOpen, setIsMobileSidebarOpen] = React.useState(false);
  const isMobile = useIsMobile();
  const latestGeneratedReference = React.useMemo(
    () => findLatestGeneratedReference(messages),
    [messages]
  );
  const shouldUseLatestReference = shouldUseImageContextForPrompt(input);
  const activeReference =
    referenceImage ?? (shouldUseLatestReference ? latestGeneratedReference : null);

  React.useEffect(() => {
    setReferenceImage(null);
  }, [activeId]);

  // Local settings state for the toolbar
  const [settings, setImageSettings] = React.useState<ImageSettings>({});
  const currentRuntimeModel = React.useMemo(
    () =>
      imageRuntimeModels?.find(
        item =>
          item.provider === modelSelectorValue?.provider && item.model === modelSelectorValue?.model
      ),
    [imageRuntimeModels, modelSelectorValue?.model, modelSelectorValue?.provider]
  );
  const previousProfileRef = React.useRef(currentRuntimeModel?.generation_profile);

  React.useEffect(() => {
    setImageSettings(previous =>
      settingsForProfile(
        currentRuntimeModel?.generation_profile,
        previous,
        previousProfileRef.current
      )
    );
    previousProfileRef.current = currentRuntimeModel?.generation_profile;
  }, [currentRuntimeModel?.generation_profile]);

  const handleSettingsChange = React.useCallback((next: ImageSettingsPatch) => {
    setImageSettings(next);
  }, []);

  const { pendingPrompt, clearPendingPrompt } = useSysImageStore();

  // Use a ref to remember if we've left home view for the current active session
  // This prevents flickering back to home view during ID migration (draft -> server)
  const [hasStartedChat, setHasStartedChat] = React.useState(false);

  React.useEffect(() => {
    if (!activeId || messages.length === 0) {
      setHasStartedChat(false);
    } else if (messages.length > 0) {
      setHasStartedChat(true);
    }
  }, [activeId, messages.length]);

  // A conversation is considered "Home" if there's no activeId, or it's an empty draft
  const isHome =
    (!activeId || (activeId.startsWith('draft-') && messages.length === 0)) && !hasStartedChat;

  const { viewportRef, bottomRef, handleScroll } = useChatAutoFollow({
    messages,
    activeId,
  });
  const conversationSearch = React.useCallback(
    (query: string, limit: number) => controller.search?.(query, limit) ?? Promise.resolve([]),
    [controller]
  );
  const hasConversationSearch = typeof controller.search === 'function';

  React.useEffect(() => {
    if (pendingPrompt) {
      setInput(pendingPrompt);
      clearPendingPrompt();

      if (typeof window !== 'undefined') {
        const storedParams = sessionStorage.getItem('promptAssistantParams');
        if (storedParams) {
          try {
            const params = JSON.parse(storedParams);
            const newSettings = { ...settings };
            const sizeOption = currentRuntimeModel?.generation_profile.size?.options.find(
              option => option.aspect_ratio === params.ratio
            );
            if (sizeOption) newSettings.size = sizeOption.value;
            if (params.count) newSettings.count = params.count;
            // TODO: Handle model if needed
            setImageSettings(newSettings);
            sessionStorage.removeItem('promptAssistantParams');
          } catch (_e) {
            // ignore
          }
        }
      }
    }
  }, [pendingPrompt, clearPendingPrompt, setInput, settings, currentRuntimeModel]);

  const handleSendAction = React.useCallback(
    (prompt: string, reference: ImageReferenceAttachment | null) => {
      const trimmedPrompt = prompt.trim();
      if (
        (!trimmedPrompt && !reference) ||
        isSending ||
        !modelSelectorValue?.provider ||
        !modelSelectorValue.model
      ) {
        return;
      }
      if (getImagePromptCharacterCount(trimmedPrompt) > IMAGE_PROMPT_MAX_CHARACTERS) {
        toast.error(t('chat.imageInput.promptTooLong', { max: IMAGE_PROMPT_MAX_CHARACTERS }));
        return;
      }
      const outgoingPrompt = trimmedPrompt || t('chat.imageInput.defaultReferencePrompt');

      // Optimistically mark as started to prevent home view flicker
      setHasStartedChat(true);

      controller.send({
        query: outgoingPrompt,
        inputs: {
          model_config: {
            provider: modelSelectorValue.provider,
            model: modelSelectorValue.model,
            name: modelSelectorValue.model,
          },
          image_gen_config: {
            size: settings.size,
            count: settings.count,
            generation_mode: settings.generationMode,
            max_images: settings.maxImages,
          },
          ...(reference
            ? {
                image_reference: {
                  file_id: reference.fileId,
                  url: reference.url,
                  filename: reference.filename,
                  mime_type: reference.mimeType,
                },
              }
            : {}),
        },
      });
      setInput('');
      setReferenceImage(null);
    },
    [
      controller,
      isSending,
      modelSelectorValue,
      settings,
      t,
    ]
  );

  const handleSend = () => {
    handleSendAction(input, activeReference);
  };

  const handleNewChat = () => {
    if (isHome) {
      toast.info(t('chat.alreadyInDraft'));
      return;
    }
    const draft = controller.createDraft();
    controller.select(draft.id);
    setIsMobileSidebarOpen(false);
  };

  const handleSelectChat = (id: string) => {
    controller.select(id);
    setIsMobileSidebarOpen(false);
  };

  const handleReferenceImageChange = (image: ImageReferenceAttachment | null) => {
    setReferenceImage(image);
  };

  const handleUseImageAsReference = (message: ChatTypeMessage, index: number) => {
    const reference = getGeneratedImageReference(message, index);
    if (!reference) {
      toast.error(t('chat.imageInput.referenceUnavailable'));
      return;
    }
    setReferenceImage(reference);
    toast.success(t('chat.imageInput.referenceApplied'));
  };

  const handleDeleteChat = (id: string) => {
    controller.remove(id);
  };

  const handleToggleSidebar = () => {
    if (isMobile) {
      setIsMobileSidebarOpen(true);
      return;
    }
    setIsSidebarOpen(prev => !prev);
  };

  return (
    <div className="flex h-full w-full bg-background overflow-hidden font-sans">
      <div className="hidden md:block">
        <Sidebar
          isOpen={isSidebarOpen}
          activeId={activeId}
          conversations={conversationList}
          onNewChat={handleNewChat}
          onSelect={handleSelectChat}
          onDelete={handleDeleteChat}
          isHome={isHome}
          search={hasConversationSearch ? conversationSearch : undefined}
          searchKey={conversationSearchKey}
          onSelectSearchResult={result => {
            if (controller.loadAndSelect) {
              void controller.loadAndSelect(result.conversationId);
            } else {
              handleSelectChat(result.conversationId);
            }
          }}
        />
      </div>

      {/* Main Chat Area */}
      <div className="flex-1 flex flex-col relative min-w-0 bg-background">
        {/* Header */}
        <div className="h-12 sm:h-14 flex items-center justify-between px-3 sm:px-4 shrink-0 absolute top-0 left-0 right-0 z-10 bg-background/80 backdrop-blur-sm transition-opacity duration-300">
          <div className="flex items-center gap-2">
            <Button
              variant="ghost"
              isIcon
              className="h-8 w-8 text-muted-foreground"
              onClick={handleToggleSidebar}
            >
              <PanelLeft className="h-4 w-4" />
            </Button>
            {!isHome && (
              <Button
                variant="ghost"
                isIcon
                className="h-8 w-8 text-muted-foreground"
                onClick={handleNewChat}
              >
                <Plus className="h-4 w-4" />
              </Button>
            )}
          </div>
          <div
            className={cn(
              'flex items-center gap-2 transition-opacity duration-300 min-w-0',
              isHome ? 'opacity-0' : 'opacity-100'
            )}
          >
            <h2 className="font-bold text-sm sm:text-base truncate">
              {currentConversation?.title || (activeId ? t('chat.newConversation') : '')}
            </h2>
          </div>
          <div className="w-10 sm:w-16" />
        </div>

        <ChatMessageViewport
          isLoadingDetail={isLoadingDetail}
          isHome={isHome}
          messages={messages}
          viewportRef={viewportRef}
          bottomRef={bottomRef}
          onScroll={handleScroll}
          loadingFallback={<SysChatSkeleton />}
          loadingClassName="bg-background"
          showCopyButton={false}
          onUseImageAsReference={handleUseImageAsReference}
        />

        {/* Home View Layer */}
        <div
          className={cn(
            'absolute inset-0 flex items-center justify-center transition-all duration-300 ease-in-out',
            isHome && !isLoadingDetail
              ? 'opacity-100 z-0'
              : 'opacity-0 pointer-events-none -z-10 scale-95'
          )}
        >
          <ImageHomeView />
        </div>

        {/* Input Layer */}
        <div
          className={cn(
            'absolute transition-all duration-300 ease-in-out z-20 w-full pointer-events-none',
            isHome && !isLoadingDetail
              ? 'top-[58%] sm:top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 max-w-3xl px-3 sm:px-4'
              : 'bottom-2 sm:bottom-8 left-1/2 -translate-x-1/2 max-w-full sm:max-w-4xl px-3 sm:p-4 pb-[calc(env(safe-area-inset-bottom)+0.5rem)] bg-gradient-to-t from-background via-background to-transparent'
          )}
        >
          <div
            className={cn(
              'mx-auto transition-all duration-300 pointer-events-auto',
              isHome ? 'w-full' : 'w-full'
            )}
          >
            <InputArea
              input={input}
              setInput={setInput}
              isSending={isSending}
              onSend={handleSend}
              settings={settings}
              setSettings={handleSettingsChange}
              modelSelectorValue={modelSelectorValue}
              onModelChange={onModelChange}
              imageRuntimeModels={imageRuntimeModels}
              currentRuntimeModel={currentRuntimeModel}
              topNotice={inputTopNotice}
              referenceImage={referenceImage}
              hasReferenceContext={Boolean(activeReference)}
              onReferenceImageChange={handleReferenceImageChange}
            />
          </div>
        </div>
      </div>
      <Sheet open={isMobileSidebarOpen} onOpenChange={setIsMobileSidebarOpen}>
        <SheetContent side="left" className="w-full max-w-none p-0 sm:max-w-sm" showClose={false}>
          <Sidebar
            isOpen
            className="!w-full border-r-0"
            activeId={activeId}
            conversations={conversationList}
            onNewChat={handleNewChat}
            onSelect={handleSelectChat}
            onDelete={handleDeleteChat}
            onClose={() => setIsMobileSidebarOpen(false)}
            isHome={isHome}
            search={hasConversationSearch ? conversationSearch : undefined}
            searchKey={conversationSearchKey}
            onSelectSearchResult={result => {
              if (controller.loadAndSelect) {
                void controller.loadAndSelect(result.conversationId);
              } else {
                handleSelectChat(result.conversationId);
              }
            }}
          />
        </SheetContent>
      </Sheet>
    </div>
  );
}

function settingsForProfile(
  profile: ImageRuntimeModel['generation_profile'] | undefined,
  previous: ImageSettings,
  previousProfile: ImageRuntimeModel['generation_profile'] | undefined
): ImageSettings {
  const previousAspectRatio = previousProfile?.size?.options.find(
    option => option.value === previous.size
  )?.aspect_ratio;
  const matchingSize = profile?.size?.options.find(
    option => option.aspect_ratio === previousAspectRatio
  )?.value;
  const size = matchingSize ?? profile?.size?.default;
  const quantity = profile?.quantity;

  if (quantity?.mode === 'exact') {
    return { size, count: quantity.default };
  }
  if (quantity?.mode === 'sequence') {
    return { size, generationMode: 'single' };
  }
  return { size };
}

function shouldUseImageContextForPrompt(prompt: string): boolean {
  const normalized = prompt.trim().toLowerCase();
  if (!normalized) return true;
  return IMAGE_EDIT_INTENT_TERMS.some(term => normalized.includes(term));
}

const IMAGE_EDIT_INTENT_TERMS = [
  '这张',
  '这张图',
  '上一张',
  '上张',
  '原图',
  '参考图',
  '刚才',
  '继续',
  '编辑',
  '修改',
  '改成',
  '改为',
  '换成',
  '替换',
  '保持',
  '保留',
  '去掉',
  '删除',
  '加上',
  '增加',
  '添加',
  '加一个',
  '加一',
  '调整',
  '优化',
  '修复',
  '重绘',
  '局部',
  '背景换',
  '风格改',
  '把它',
  '让它',
  '它的',
  '不要',
  '变成',
  'this image',
  'previous image',
  'last image',
  'reference image',
  'edit',
  'modify',
  'change',
  'replace',
  'keep',
  'preserve',
  'remove',
  'delete',
  'add',
  'continue',
  'based on',
  'make it',
  'turn it',
];

function getGeneratedImageReference(
  message: ChatTypeMessage,
  index: number
): ImageReferenceAttachment | null {
  const generatedImage = message.generatedImages?.[index];
  const metadata =
    message.messageData?.metadata && typeof message.messageData.metadata === 'object'
      ? (message.messageData.metadata as Record<string, unknown>)
      : undefined;
  const imageGeneration =
    message.messageData?.image_generation &&
    typeof message.messageData.image_generation === 'object'
      ? (message.messageData.image_generation as Record<string, unknown>)
      : metadata?.image_generation && typeof metadata.image_generation === 'object'
        ? (metadata.image_generation as Record<string, unknown>)
        : undefined;
  const files = Array.isArray(imageGeneration?.files) ? imageGeneration.files : [];
  const file = files[index];
  const data = file && typeof file === 'object' ? (file as Record<string, unknown>) : {};
  const fileId =
    generatedImage?.fileId?.trim() ||
    readString(data, 'file_id', 'tool_file_id', 'upload_file_id', 'id');
  const url =
    generatedImage?.url?.trim() ||
    readString(data, 'url', 'download_url');
  if (!fileId || !url) return null;

  return {
    fileId,
    url,
    filename:
      generatedImage?.filename?.trim() ||
      readString(data, 'filename', 'name'),
    mimeType:
      generatedImage?.mimeType?.trim() ||
      readString(data, 'mime_type', 'content_type') ||
      'image/*',
  };
}

function findLatestGeneratedReference(
  messages: ChatTypeMessage[]
): ImageReferenceAttachment | null {
  for (let messageIndex = messages.length - 1; messageIndex >= 0; messageIndex -= 1) {
    const message = messages[messageIndex];
    const images = message.generatedImages ?? [];
    for (let imageIndex = images.length - 1; imageIndex >= 0; imageIndex -= 1) {
      if (images[imageIndex]?.isLoading) continue;
      const reference = getGeneratedImageReference(message, imageIndex);
      if (reference) return reference;
    }
  }
  return null;
}

function readString(value: Record<string, unknown>, ...keys: string[]): string {
  for (const key of keys) {
    if (typeof value[key] === 'string' && value[key].trim()) {
      return value[key].trim();
    }
  }
  return '';
}
