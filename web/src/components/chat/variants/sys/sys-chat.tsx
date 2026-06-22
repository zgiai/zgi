'use client';

import * as React from 'react';
import { PanelLeft, Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import type { ChatController } from '@/components/chat/controllers/types';
import { useChatStore } from '@/components/chat/store';
import { useT } from '@/i18n/translations';
import { InputArea } from './input-area';
import { Sidebar } from '../common/sidebar';
import type { Message as ChatTypeMessage } from '@/components/chat/types';
import { cn } from '@/lib/utils';
import { SysHomeView } from './home-view';
import { SysChatSkeleton } from './skeleton';
import { toast } from 'sonner';
import { useStore } from 'zustand';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import { useIsMobile } from '@/hooks/use-mobile';
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet';
import { useChatAutoFollow } from '../common/use-chat-auto-follow';
import { ChatMessageViewport } from '../common/chat-message-viewport';

export interface SysChatProps {
  controller: ChatController;
  className?: string;
  modelSelectorValue?: ModelSelectorValue;
  onModelChange?: (value: ModelSelectorValue) => void;
  extraInputs?: Record<string, unknown>;
  historyWindowSize?: number;
  inputTopNotice?: React.ReactNode;
  conversationSearchKey?: readonly unknown[];
}

export function SysChat({
  controller,
  modelSelectorValue,
  onModelChange,
  extraInputs,
  historyWindowSize,
  inputTopNotice,
  conversationSearchKey,
}: SysChatProps) {
  const activeId = useStore(controller.store, s => s.activeId);
  const conversationList = useStore(controller.store, s => s.conversations);
  const isSending = useStore(controller.store, s => s.isSending);
  const isLoadingDetail = useStore(controller.store, s => s.isLoadingDetail);
  const t = useT('webapp');

  const activeConversationStore = useChatStore(state =>
    activeId ? state.conversations[activeId] : undefined
  );

  const [input, setInput] = React.useState('');
  const [isSidebarOpen, setIsSidebarOpen] = React.useState(true);
  const [isMobileSidebarOpen, setIsMobileSidebarOpen] = React.useState(false);
  const isMobile = useIsMobile();

  // Map store messages to SysChat messages
  const messages: ChatTypeMessage[] = React.useMemo(() => {
    if (!activeConversationStore) return [];
    return activeConversationStore.messages;
  }, [activeConversationStore]);

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

  const handleSend = React.useCallback(async () => {
    const trimmedInput = input.trim();
    if (!trimmedInput || isSending) return;

    // Clear input immediately for better responsiveness
    setInput('');
    // Optimistically mark as started to prevent home view flicker
    setHasStartedChat(true);

    controller.send({
      query: trimmedInput,
      inputs: extraInputs,
      historyWindowSize: typeof historyWindowSize === 'number' ? historyWindowSize : undefined,
    });
  }, [input, isSending, controller, extraInputs, historyWindowSize]);

  const handleNewChat = () => {
    if (isHome) {
      toast.info(t('chat.alreadyInDraft'));
      return;
    }
    const draft = controller.createDraft();
    controller.select(draft.id);
    setIsMobileSidebarOpen(false);
  };

  const handleDeleteChat = (id: string) => {
    controller.remove(id);
  };

  const handleSelectChat = (id: string) => {
    controller.select(id);
    setIsMobileSidebarOpen(false);
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
        />
      </div>

      {/* Main Chat Area */}
      <div className="flex-1 flex flex-col min-w-0 bg-white relative">
        {/* Header */}
        <div className="h-12 sm:h-14 flex items-center justify-between px-3 sm:px-4 shrink-0 absolute top-0 left-0 right-0 z-10 bg-white/80 backdrop-blur-sm transition-opacity duration-300">
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
              {activeConversationStore?.title || (activeId ? t('chat.newConversation') : '')}
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
          loadingClassName="bg-white"
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
          <SysHomeView onSuggestionClick={text => setInput(text)} />
        </div>

        {/* Input Layer - Animatable */}
        <div
          className={cn(
            'absolute transition-all duration-300 ease-in-out z-20 w-full pointer-events-none',
            isHome && !isLoadingDetail
              ? 'top-[58%] sm:top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 max-w-3xl px-3 sm:px-4'
              : 'bottom-2 sm:bottom-8 left-1/2 -translate-x-1/2 max-w-full sm:max-w-4xl px-3 sm:p-4 pb-[calc(env(safe-area-inset-bottom)+0.5rem)] bg-gradient-to-t from-white via-white to-transparent'
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
              modelSelectorValue={modelSelectorValue}
              onModelChange={onModelChange}
              topNotice={inputTopNotice}
            />
          </div>
        </div>
        {!isHome && !isLoadingDetail && (
          <div className="absolute bottom-[calc(env(safe-area-inset-bottom)+0.25rem)] sm:bottom-4 left-1/2 -translate-x-1/2 text-center text-[11px] text-muted-foreground/60 font-medium">
            {t('chat.aiDisclaimer')}
          </div>
        )}
      </div>
      <Sheet open={isMobileSidebarOpen} onOpenChange={setIsMobileSidebarOpen}>
        <SheetContent side="left" className="max-w-none p-0 sm:max-w-sm" showClose={false}>
          <SheetTitle className="sr-only" />
          <Sidebar
            isOpen
            className="w-full border-r-0"
            activeId={activeId}
            conversations={conversationList}
            onNewChat={handleNewChat}
            onSelect={handleSelectChat}
            onDelete={handleDeleteChat}
            onClose={() => setIsMobileSidebarOpen(false)}
            isHome={isHome}
            search={hasConversationSearch ? conversationSearch : undefined}
            searchKey={conversationSearchKey}
          />
        </SheetContent>
      </Sheet>
    </div>
  );
}
