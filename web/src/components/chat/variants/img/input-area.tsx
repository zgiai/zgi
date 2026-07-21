import * as React from 'react';
import { ArrowUp } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n/translations';
import { SettingsToolbar, type ImageSettings, type ImageSettingsPatch } from './settings-toolbar';
import { ModelSelector, type ModelSelectorValue } from '@/components/common/model-selector';
import type { ImageRuntimeModel } from '@/services/types/image-runtime';
import type { ModelItem } from '@/services/types/model';

interface InputAreaProps {
  input: string;
  setInput: (input: string) => void;
  isSending: boolean;
  onSend: () => void;
  settings: ImageSettings;
  setSettings: (settings: ImageSettingsPatch) => void;
  modelSelectorValue?: ModelSelectorValue;
  onModelChange?: (value: ModelSelectorValue) => void;
  imageRuntimeModels?: ImageRuntimeModel[];
  ratioOptions?: string[];
  countOptions?: number[];
  topNotice?: React.ReactNode;
}

export function InputArea({
  input,
  setInput,
  isSending,
  onSend,
  settings,
  setSettings,
  modelSelectorValue,
  onModelChange,
  imageRuntimeModels,
  ratioOptions,
  countOptions,
  topNotice,
}: InputAreaProps) {
  const t = useT('webapp');
  const textareaRef = React.useRef<HTMLTextAreaElement>(null);
  const imageRuntimeModelItems = React.useMemo(
    () => imageRuntimeModels?.map(mapImageRuntimeModelToModelItem),
    [imageRuntimeModels]
  );

  const adjustHeight = React.useCallback(() => {
    const textarea = textareaRef.current;
    if (textarea) {
      textarea.style.height = 'auto';
      // 24px line-height * 5 lines = 120px + padding
      const maxHeight = 120 + 24;
      const scrollHeight = textarea.scrollHeight;

      if (scrollHeight > maxHeight) {
        textarea.style.height = `${maxHeight}px`;
        textarea.style.overflowY = 'auto';
      } else {
        textarea.style.height = `${scrollHeight}px`;
        textarea.style.overflowY = 'hidden';
      }
    }
  }, []);

  React.useEffect(() => {
    // Initial adjust
    adjustHeight();
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
        <div className="flex items-center gap-2 min-w-0 flex-1 flex-wrap">
          {onModelChange ? (
            <div className="w-[140px] sm:w-[180px] shrink-0">
              <ModelSelector
                modelType="image-gen"
                value={modelSelectorValue}
                onChange={onModelChange}
                modelsOverride={imageRuntimeModelItems}
                className="h-[26px] rounded-md border-border/50 bg-background px-2 text-xs font-medium hover:bg-muted/40 hover:border-border shadow-sm"
                showCapabilities={false}
              />
            </div>
          ) : null}
          <SettingsToolbar
            onSettingsChange={setSettings}
            initialSettings={settings}
            ratioOptions={ratioOptions}
            countOptions={countOptions}
          />
        </div>

        <div className="flex items-center gap-1 shrink-0">
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

function mapImageRuntimeModelToModelItem(item: ImageRuntimeModel): ModelItem {
  const now = Math.floor(Date.now() / 1000);
  const label = item.model_label || item.model;

  return {
    id: `${item.provider}/${item.model}`,
    provider: item.provider,
    model: item.model,
    model_name: label,
    family: item.model,
    family_name: label,
    status: 'active',
    tagline: '',
    is_flagship: false,
    is_recommended: false,
    is_featured: false,
    is_new: false,
    access_type: 'open',
    currency: '',
    input_price: 0,
    output_price: 0,
    context_window: 0,
    max_output_tokens: 0,
    endpoints: { image: true },
    features: {},
    tools: {},
    use_cases: ['image-gen'],
    input_modalities: ['text'],
    output_modalities: ['image'],
    is_enabled: true,
    is_available: true,
    is_configured: true,
    callable: true,
    tier: '',
    created_at: now,
    updated_at: now,
  };
}
