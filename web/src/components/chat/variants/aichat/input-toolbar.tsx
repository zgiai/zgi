'use client';

import {
  FileText,
  FolderOpen,
  ImageIcon,
  Info,
  Maximize2,
  Minimize2,
  Paperclip,
  Send,
  Square,
} from 'lucide-react';
import {
  ModelSelector,
  type ModelSelectorModelProps,
  type ModelSelectorValue,
} from '@/components/common/model-selector';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { formatExtensionsForDisplay } from '@/utils/file-helpers';
import { AIChatMemoryModule } from '@/components/chat/variants/aichat/memory-module';
import type { AIChatModelValue } from '@/components/chat/variants/aichat/types';
import {
  imageAttachmentHintTranslationKey,
  type ScopedTranslatorWithHas,
  tAttachmentForSurface,
  type AIChatComposerSurface,
} from '@/components/chat/variants/aichat/input-area-utils';

interface AIChatInputToolbarProps {
  modelSelectorValue: AIChatModelValue;
  isModelInitializing?: boolean;
  modelMissing: boolean;
  modelCapabilityFilter?: { features_vision: boolean };
  hasImageAttachment: boolean;
  isSending: boolean;
  canStop?: boolean;
  isUploading: boolean;
  isStopping: boolean;
  canSend: boolean;
  canUseImage: boolean;
  remainingSlots: number;
  attachmentLimit: number;
  maxSizeMB: number;
  imageMaxSizeMB: number;
  allowedExtensions: string[];
  imageExtensions: string[];
  showModelSelector?: boolean;
  showMemoryToggle?: boolean;
  showComposerExpandButton?: boolean;
  isComposerExpanded?: boolean;
  enableUpload?: boolean;
  showFileLibraryPicker?: boolean;
  surface?: AIChatComposerSurface;
  onModelChange: (value: ModelSelectorValue) => void;
  onModelPropsChange: (model: ModelSelectorModelProps | null) => void;
  onUploadDocument: () => void;
  onUploadImage: () => void;
  onSelectFromFiles: () => void;
  onMemoryEnabledChange: (enabled: boolean) => void;
  onToggleComposerExpanded?: () => void;
  onSend: () => void | Promise<void>;
  onStop: () => void;
}

/**
 * @component AIChatInputToolbar
 * @category Feature
 * @status Stable
 * @description Bottom action row for AIChat composer model, attachment, and send controls.
 * @usage Render inside AIChatInputArea below the textarea
 * @example
 * <AIChatInputToolbar modelSelectorValue={value} canSend onSend={send} />
 */
export function AIChatInputToolbar({
  modelSelectorValue,
  isModelInitializing = false,
  modelMissing,
  modelCapabilityFilter,
  hasImageAttachment,
  isSending,
  canStop,
  isUploading,
  isStopping,
  canSend,
  canUseImage,
  remainingSlots,
  attachmentLimit,
  maxSizeMB,
  imageMaxSizeMB,
  allowedExtensions,
  imageExtensions,
  showModelSelector = true,
  showMemoryToggle = true,
  showComposerExpandButton = false,
  isComposerExpanded = false,
  enableUpload = true,
  showFileLibraryPicker = true,
  surface = 'aichat',
  onModelChange,
  onModelPropsChange,
  onUploadDocument,
  onUploadImage,
  onSelectFromFiles,
  onMemoryEnabledChange,
  onToggleComposerExpanded,
  onSend,
  onStop,
}: AIChatInputToolbarProps) {
  const t = useT('webapp');
  const showStopButton = canStop ?? isSending;

  return (
    <div className="flex items-center justify-between gap-2 px-1">
      <div className="flex min-w-0 items-center gap-1.5">
        {showModelSelector ? (
          <div className="w-44 min-w-0 sm:w-56">
            {isModelInitializing ? (
              <div className="h-8 rounded-full border border-border/70 bg-muted/50" />
            ) : (
              <ModelSelector
                modelType="text-chat"
                value={modelSelectorValue}
                onChange={onModelChange}
                onModelPropsChange={onModelPropsChange}
                capabilityFilter={modelCapabilityFilter}
                className={cn(
                  'h-8 rounded-full border-border/70 px-3 text-xs',
                  modelMissing ? 'border-destructive/50' : ''
                )}
                showCapabilities={false}
              />
            )}
          </div>
        ) : null}
        {hasImageAttachment ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <Info className="size-4 shrink-0 text-muted-foreground" />
            </TooltipTrigger>
            <TooltipContent side="top" className="max-w-xs">
              {tAttachmentForSurface(
                t as unknown as ScopedTranslatorWithHas,
                surface,
                imageAttachmentHintTranslationKey(surface),
                'imageModelLocked'
              )}
            </TooltipContent>
          </Tooltip>
        ) : null}
      </div>
      <div className="flex shrink-0 items-center gap-1">
        {showMemoryToggle ? (
          <AIChatMemoryModule disabled={isSending} onEnabledChange={onMemoryEnabledChange} />
        ) : null}
        {enableUpload ? (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                isIcon
                variant="ghost"
                className="size-8 rounded-full"
                disabled={isSending || isUploading || remainingSlots <= 0}
                title={t('consoleChat.attachments.upload')}
              >
                <Paperclip className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-48">
              <Tooltip>
                <TooltipTrigger asChild>
                  <div>
                    <DropdownMenuItem onClick={onUploadDocument}>
                      <FileText className="mr-2 size-4" />
                      {t('consoleChat.attachments.uploadDocument')}
                    </DropdownMenuItem>
                  </div>
                </TooltipTrigger>
                <TooltipContent side="left" className="max-w-xs">
                  {t('consoleChat.attachments.uploadDocumentTooltip', {
                    count: attachmentLimit,
                    size: maxSizeMB,
                    types: formatExtensionsForDisplay(allowedExtensions).join(' / '),
                  })}
                </TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <div>
                    <DropdownMenuItem disabled={!canUseImage} onClick={onUploadImage}>
                      <ImageIcon className="mr-2 size-4" />
                      {t('consoleChat.attachments.uploadImage')}
                    </DropdownMenuItem>
                  </div>
                </TooltipTrigger>
                <TooltipContent side="left" className="max-w-xs">
                  {canUseImage
                    ? tAttachmentForSurface(
                        t as unknown as ScopedTranslatorWithHas,
                        surface,
                        'uploadImageTooltip',
                        'uploadImageTooltip',
                        {
                          count: attachmentLimit,
                          size: imageMaxSizeMB,
                          types: formatExtensionsForDisplay(imageExtensions).join(' / '),
                        }
                      )
                    : tAttachmentForSurface(
                        t as unknown as ScopedTranslatorWithHas,
                        surface,
                        'imageVisionRequired'
                      )}
                </TooltipContent>
              </Tooltip>
              {showFileLibraryPicker ? (
                <DropdownMenuItem onClick={onSelectFromFiles}>
                  <FolderOpen className="mr-2 size-4" />
                  {t('consoleChat.attachments.selectFromFiles')}
                </DropdownMenuItem>
              ) : null}
            </DropdownMenuContent>
          </DropdownMenu>
        ) : null}
        {showComposerExpandButton && onToggleComposerExpanded ? (
          <Button
            type="button"
            isIcon
            variant="ghost"
            className="size-8 rounded-full"
            onClick={onToggleComposerExpanded}
            title={t(
              isComposerExpanded
                ? 'consoleChat.composer.collapseEditor'
                : 'consoleChat.composer.expandEditor'
            )}
            aria-label={t(
              isComposerExpanded
                ? 'consoleChat.composer.collapseEditor'
                : 'consoleChat.composer.expandEditor'
            )}
            aria-pressed={isComposerExpanded}
          >
            {isComposerExpanded ? (
              <Minimize2 className="size-4" />
            ) : (
              <Maximize2 className="size-4" />
            )}
          </Button>
        ) : null}
        {showStopButton ? (
          <Button
            isIcon
            variant="destructive"
            className="size-8 rounded-full"
            disabled={isStopping}
            onClick={onStop}
            title={t('consoleChat.stop')}
          >
            <Square className="size-3.5 fill-current" />
          </Button>
        ) : (
          <Button
            isIcon
            className="size-8 rounded-full"
            disabled={!canSend}
            onClick={onSend}
            title={t('consoleChat.send')}
          >
            <Send className="size-4" />
          </Button>
        )}
      </div>
    </div>
  );
}
