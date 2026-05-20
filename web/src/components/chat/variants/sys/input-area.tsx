import * as React from 'react';
import {
  // Sparkles,
  // Settings2,
  // Paperclip,
  // Globe,
  // Brain,
  // Mic,
  ArrowUp,
  // ChevronDown,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
// import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n/translations';
import { ModelSelector, type ModelSelectorValue } from '@/components/common/model-selector';

interface InputAreaProps {
  input: string;
  setInput: (input: string) => void;
  isSending: boolean;
  onSend: () => void;
  modelSelectorValue?: ModelSelectorValue;
  onModelChange?: (value: ModelSelectorValue) => void;
  topNotice?: React.ReactNode;
}

export function InputArea({
  input,
  setInput,
  isSending,
  onSend,
  modelSelectorValue,
  onModelChange,
  topNotice,
}: InputAreaProps) {
  const t = useT('webapp');
  const textareaRef = React.useRef<HTMLTextAreaElement>(null);

  const adjustHeight = React.useCallback(() => {
    const textarea = textareaRef.current;
    if (textarea) {
      textarea.style.height = 'auto';
      const newHeight = Math.min(Math.max(textarea.scrollHeight, 48), 144);
      textarea.style.height = `${newHeight}px`;

      // Control overflow to prevent scrollbar flash
      if (textarea.scrollHeight > 144) {
        textarea.style.overflowY = 'auto';
      } else {
        textarea.style.overflowY = 'hidden';
      }
    }
  }, []);

  // Listen for width changes (layout transitions) to adjust height
  React.useEffect(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;

    const observer = new ResizeObserver(() => {
      adjustHeight();
    });

    observer.observe(textarea);
    return () => observer.disconnect();
  }, [adjustHeight]);

  React.useEffect(() => {
    adjustHeight();
  }, [input, adjustHeight]);

  return (
    <div className="relative border border-border rounded-[24px] p-2 shadow-sm bg-background hover:border-primary/20 focus-within:border-primary/20 transition-colors w-full">
      {topNotice}
      <Textarea
        ref={textareaRef}
        value={input}
        onChange={e => {
          setInput(e.target.value);
        }}
        onKeyDown={e => {
          if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            onSend();
          }
        }}
        placeholder={t('chat.enterCommand')}
        className={cn(
          'w-full resize-none border-0 focus-visible:ring-0 px-3 py-2 text-base shadow-none bg-transparent',
          'overflow-y-hidden hover:overflow-y-auto pr-2',
          'scrollbar-thin scrollbar-thumb-muted-foreground/20 hover:scrollbar-thumb-muted-foreground/40 scrollbar-track-transparent'
        )}
        style={{ minHeight: '48px', maxHeight: '144px' }}
      />

      {/* Toolbar */}
      <div className="flex items-end justify-between gap-2 px-1 sm:px-2 pt-2">
        <div className="flex items-center gap-2 min-w-0 flex-1">
          {onModelChange && (
            <div className="w-[140px] sm:w-[180px] shrink-0">
              <ModelSelector
                modelType="text-chat"
                value={modelSelectorValue}
                onChange={onModelChange}
                className="h-8 rounded-full border-border/60 text-xs font-medium hover:bg-muted/50 px-3"
                showCapabilities={false}
              />
            </div>
          )}
          {/* <Button
            variant="ghost"
            isIcon
            className="h-8 w-8 rounded-full text-muted-foreground hover:bg-muted/50"
          >
            <Settings2 className="h-4 w-4" />
          </Button> */}
        </div>

        <div className="flex items-center gap-1 shrink-0">
          {/* <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  isIcon
                  className="h-8 w-8 rounded-full text-muted-foreground hover:bg-muted/50"
                >
                  <Paperclip className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Attach</TooltipContent>
            </Tooltip>
          </TooltipProvider>

          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  isIcon
                  className="h-8 w-8 rounded-full text-muted-foreground hover:bg-muted/50"
                >
                  <Globe className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Search</TooltipContent>
            </Tooltip>
          </TooltipProvider>

          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  isIcon
                  className="h-8 w-8 rounded-full text-muted-foreground hover:bg-muted/50"
                >
                  <Brain className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Thinking</TooltipContent>
            </Tooltip>
          </TooltipProvider>

          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  isIcon
                  className="h-8 w-8 rounded-full text-muted-foreground hover:bg-muted/50"
                >
                  <Mic className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Voice</TooltipContent>
            </Tooltip>
          </TooltipProvider> */}

          <Button
            isIcon
            className={cn(
              'h-8 w-8 rounded-full ml-1 transition-all',
              input.trim()
                ? 'bg-primary text-white hover:bg-primary-hover'
                : 'bg-muted text-muted-foreground'
            )}
            onClick={onSend}
            disabled={!input.trim() || isSending}
          >
            <ArrowUp className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}
