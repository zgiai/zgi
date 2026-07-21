'use client';

import * as React from 'react';
import { useChatStore } from '@/components/chat/store';
import { InputArea } from './input-area';
import { ImageHomeView } from './home-view';
import { Sidebar } from '../common/sidebar';
import { Loader2, PanelLeft, Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import type { ChatController } from '@/components/chat/controllers/types';
import { IMAGE_COUNTS } from './constants';
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

const LONG_IMAGE_GENERATION_NOTICE_DELAY_MS = 30000;
const LONG_IMAGE_GENERATION_NOTICE_TEXT = '图像仍在生成中，高清或竖版图片可能需要几分钟，请稍候。';

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
  const [isSidebarOpen, setIsSidebarOpen] = React.useState(true);
  const [isMobileSidebarOpen, setIsMobileSidebarOpen] = React.useState(false);
  const [showLongGenerationNotice, setShowLongGenerationNotice] = React.useState(false);
  const isMobile = useIsMobile();

  // Local settings state for the toolbar
  const [settings, setImageSettings] = React.useState<ImageSettings>({
    ratio: '1:1',
    count: IMAGE_COUNTS[0].id,
    customRatio: { width: 1024, height: 1024 },
    isCustomRatio: false,
  });
  const currentRuntimeModel = React.useMemo(
    () =>
      imageRuntimeModels?.find(
        item => item.provider === modelSelectorValue?.provider && item.model === modelSelectorValue?.model
      ),
    [imageRuntimeModels, modelSelectorValue?.model, modelSelectorValue?.provider]
  );
  const ratioOptions = React.useMemo(
    () => Array.from(new Set(currentRuntimeModel?.supported_sizes.map(sizeToRatio).filter(Boolean))),
    [currentRuntimeModel?.supported_sizes]
  );

  const handleSettingsChange = React.useCallback((next: ImageSettingsPatch) => {
    setImageSettings(prev => ({
      ...prev,
      ratio: next.ratio,
      count: next.count,
      customRatio: next.customRatio ?? prev.customRatio,
      isCustomRatio: next.isCustomRatio ?? prev.isCustomRatio,
    }));
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

  React.useEffect(() => {
    if (!isSending) {
      setShowLongGenerationNotice(false);
      return;
    }
    const timer = window.setTimeout(() => {
      setShowLongGenerationNotice(true);
    }, LONG_IMAGE_GENERATION_NOTICE_DELAY_MS);
    return () => window.clearTimeout(timer);
  }, [isSending]);

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
            if (params.isCustom && params.customSize) {
              newSettings.isCustomRatio = true;
              newSettings.customRatio = params.customSize;
            } else if (params.ratio) {
              newSettings.ratio = params.ratio;
              newSettings.isCustomRatio = false;
            }
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
  }, [pendingPrompt, clearPendingPrompt, setInput, settings, setImageSettings]);

  const handleSendAction = React.useCallback(
    (prompt: string) => {
      if (!prompt.trim() || isSending) return;

      // Optimistically mark as started to prevent home view flicker
      setHasStartedChat(true);

      controller.send({
        query: prompt,
        inputs: {
          ...(modelSelectorValue?.provider && modelSelectorValue?.model
            ? {
                model_config: {
                  provider: modelSelectorValue.provider,
                  model: modelSelectorValue.model,
                  name: modelSelectorValue.model,
                },
              }
            : {}),
          image_gen_config: {
            aspect_ratio: settings.isCustomRatio ? '1:1' : settings.ratio,
            n: settings.count,
          },
        },
      });
      setInput('');
    },
    [controller, isSending, settings, modelSelectorValue]
  );

  const handleSend = () => {
    handleSendAction(input);
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

  const generationNotice = showLongGenerationNotice ? (
    <div className="flex w-full justify-start">
      <div className="flex max-w-[min(720px,100%)] items-center gap-2 rounded-lg bg-muted/60 px-3 py-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 shrink-0 animate-spin" />
        <span>{LONG_IMAGE_GENERATION_NOTICE_TEXT}</span>
      </div>
    </div>
  ) : null;

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
          trailingContent={generationNotice}
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
              ratioOptions={ratioOptions}
              countOptions={currentRuntimeModel?.supported_counts}
              topNotice={inputTopNotice}
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

function sizeToRatio(size: string): string {
  switch (size) {
    case '1536x1024':
      return '3:2';
    case '1024x1536':
      return '2:3';
    case '1792x1024':
    case '2048x1152':
    case '3840x2160':
      return '16:9';
    case '1024x1792':
    case '2160x3840':
      return '9:16';
    case '1024x768':
      return '4:3';
    case '1024x1024':
    case '2048x2048':
      return '1:1';
    default:
      return '';
  }
}
