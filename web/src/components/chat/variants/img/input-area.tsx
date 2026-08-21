import * as React from 'react';
import { ArrowUp, FolderOpen, Loader2, Paperclip, Upload, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n/translations';
import { SettingsToolbar, type ImageSettings, type ImageSettingsPatch } from './settings-toolbar';
import { ModelSelector, type ModelSelectorValue } from '@/components/common/model-selector';
import type { ImageRuntimeModel } from '@/services/types/image-runtime';
import type { ModelItem } from '@/services/types/model';
import FileSelectorDialog from '@/components/files/file-selector-dialog';
import type { FileItem } from '@/services/types/file';
import { fileManageService, uploadService } from '@/services';
import type { UploadResponse } from '@/services/upload.service';
import { toast } from 'sonner';

const IMAGE_REFERENCE_ACCEPT_EXTENSIONS = ['jpg', 'jpeg', 'png', 'webp', 'gif'];

export interface ImageReferenceAttachment {
  fileId: string;
  url: string;
  filename: string;
  mimeType: string;
}

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
  currentRuntimeModel?: ImageRuntimeModel;
  topNotice?: React.ReactNode;
  referenceImage?: ImageReferenceAttachment | null;
  onReferenceImageChange?: (image: ImageReferenceAttachment | null) => void;
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
  currentRuntimeModel,
  topNotice,
  referenceImage,
  onReferenceImageChange,
}: InputAreaProps) {
  const t = useT('webapp');
  const textareaRef = React.useRef<HTMLTextAreaElement>(null);
  const fileInputRef = React.useRef<HTMLInputElement>(null);
  const [isReferenceUploading, setIsReferenceUploading] = React.useState(false);
  const [isFileSelectorOpen, setIsFileSelectorOpen] = React.useState(false);
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
      {referenceImage ? (
        <div className="px-2 pt-1">
          <div className="group relative inline-flex h-16 max-w-full items-center rounded-[4px] border bg-muted/30 p-1 pr-8">
            <img
              src={referenceImage.url}
              alt={referenceImage.filename || t('chat.imageInput.referenceImage')}
              className="h-14 max-w-[112px] rounded-[3px] object-contain"
            />
            <Button
              type="button"
              isIcon
              variant="ghost"
              className="absolute right-1 top-1 h-6 w-6 rounded-full bg-background/80"
              onClick={() => onReferenceImageChange?.(null)}
              disabled={isSending || isReferenceUploading}
              aria-label={t('chat.imageInput.removeReference')}
            >
              <X className="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>
      ) : null}
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
                emptyStateTitle={t('chat.imageGenEmpty.title')}
                emptyStateDescription={t('chat.imageGenEmpty.description')}
                className="h-[26px] rounded-md border-border/50 bg-background px-2 text-xs font-medium hover:bg-muted/40 hover:border-border shadow-sm"
                showCapabilities={false}
              />
            </div>
          ) : null}
          <SettingsToolbar
            onSettingsChange={setSettings}
            settings={settings}
            profile={currentRuntimeModel?.generation_profile}
          />
        </div>

        <div className="flex items-center gap-1 shrink-0">
          <input
            ref={fileInputRef}
            type="file"
            accept="image/*"
            className="hidden"
            onChange={event => {
              const file = event.target.files?.[0];
              event.target.value = '';
              if (file) {
                void uploadReferenceFile(file);
              }
            }}
          />
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                isIcon
                className="h-8 w-8 rounded-full text-muted-foreground hover:bg-muted"
                disabled={isSending || isReferenceUploading}
                aria-label={
                  referenceImage
                    ? t('chat.imageInput.replaceReference')
                    : t('chat.imageInput.attachReference')
                }
              >
                {isReferenceUploading ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Paperclip className="h-4 w-4" />
                )}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onSelect={() => fileInputRef.current?.click()}>
                <Upload className="h-4 w-4" />
                {t('chat.imageInput.uploadImage')}
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => setIsFileSelectorOpen(true)}>
                <FolderOpen className="h-4 w-4" />
                {t('chat.imageInput.selectFromFiles')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
          <Button
            isIcon
            className={cn(
              'h-8 w-8 rounded-full ml-1 transition-all',
              input.trim() || referenceImage
                ? 'bg-primary text-white hover:bg-primary-hover'
                : 'bg-muted text-muted-foreground'
            )}
            onClick={onSend}
            disabled={
              (!input.trim() && !referenceImage) ||
              isSending ||
              isReferenceUploading ||
              !modelSelectorValue?.model
            }
          >
            <ArrowUp className="h-4 w-4" />
          </Button>
        </div>
      </div>
      <FileSelectorDialog
        open={isFileSelectorOpen}
        onOpenChange={setIsFileSelectorOpen}
        maxCount={1}
        acceptExt={IMAGE_REFERENCE_ACCEPT_EXTENSIONS}
        initSelectedFiles={[]}
        onConfirm={files => {
          const file = files[0];
          if (file) {
            void selectManagedReferenceFile(file);
          }
        }}
      />
    </div>
  );

  async function uploadReferenceFile(file: File) {
    if (!file.type.startsWith('image/')) {
      toast.error(t('chat.imageInput.imageOnly'));
      return;
    }
    setIsReferenceUploading(true);
    try {
      const uploaded = await uploadService.uploadSingle(file, {
        is_temporary: true,
        processing_mode: 'store_only',
      });
      const previewURL = await resolveUploadedImageURL(uploaded);
      onReferenceImageChange?.({
        fileId: uploaded.id,
        url: previewURL,
        filename: uploaded.name || file.name,
        mimeType: uploaded.mime_type || file.type,
      });
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('chat.imageInput.uploadFailed'));
    } finally {
      setIsReferenceUploading(false);
    }
  }

  async function selectManagedReferenceFile(file: FileItem) {
    if (!isImageFile(file.mime_type, file.extension)) {
      toast.error(t('chat.imageInput.imageOnly'));
      return;
    }
    setIsReferenceUploading(true);
    try {
      const preview = await fileManageService.getOriginalPreviewUrl(file.id);
      const previewURL = preview.data?.url?.trim();
      if (!previewURL) {
        throw new Error(t('chat.imageInput.previewUrlMissing'));
      }
      onReferenceImageChange?.({
        fileId: file.id,
        url: previewURL,
        filename: file.name,
        mimeType: file.mime_type,
      });
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('chat.imageInput.selectFailed'));
    } finally {
      setIsReferenceUploading(false);
    }
  }

  async function resolveUploadedImageURL(uploaded: UploadResponse): Promise<string> {
    const directURL = uploaded.source_url?.trim() || uploaded.url?.trim() || uploaded.download_url?.trim();
    if (directURL) return directURL;
    const preview = await fileManageService.getOriginalPreviewUrl(uploaded.id);
    const previewURL = preview.data?.url?.trim();
    if (previewURL) return previewURL;
    throw new Error(t('chat.imageInput.previewUrlMissing'));
  }
}

function isImageFile(mimeType: string, extension: string): boolean {
  if (mimeType.trim().toLowerCase().startsWith('image/')) return true;
  return IMAGE_REFERENCE_ACCEPT_EXTENSIONS.includes(extension.trim().toLowerCase().replace(/^\./, ''));
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
