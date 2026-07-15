'use client';

import {
  ChevronDown,
  FileText,
  FolderOpen,
  ImageIcon,
  Info,
  Maximize2,
  Minimize2,
  Paperclip,
  Send,
  Settings2,
  Shield,
  ShieldAlert,
  ShieldCheck,
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
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useT, type ScopedTranslations } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { formatExtensionsForDisplay } from '@/utils/file-helpers';
import { AIChatMemoryModule } from '@/components/chat/variants/aichat/memory-module';
import type { AIChatModelValue } from '@/components/chat/variants/aichat/types';
import type { AIChatToolGovernancePermissionTier } from '@/components/aichat/contextual/types';
import type { ModelUseCase } from '@/services/types/model';
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
  modelUseCase?: ModelUseCase;
  preferredModelUseCase?: ModelUseCase;
  showMemoryToggle?: boolean;
  showComposerExpandButton?: boolean;
  isComposerExpanded?: boolean;
  showToolGovernancePermissionControl?: boolean;
  toolGovernancePermissionTier?: AIChatToolGovernancePermissionTier;
  enableUpload?: boolean;
  showFileLibraryPicker?: boolean;
  showSkillManagement?: boolean;
  skillManagementLabel?: string;
  surface?: AIChatComposerSurface;
  onModelChange: (value: ModelSelectorValue) => void;
  onModelPropsChange: (model: ModelSelectorModelProps | null) => void;
  onUploadDocument: () => void;
  onUploadImage: () => void;
  onSelectFromFiles: () => void;
  onOpenSkillManagement?: () => void;
  onMemoryEnabledChange: (enabled: boolean) => void;
  onToggleComposerExpanded?: () => void;
  onToolGovernancePermissionTierChange?: (tier: AIChatToolGovernancePermissionTier) => void;
  onSend: () => void | Promise<void>;
  onStop: () => void;
}

const TOOL_GOVERNANCE_PERMISSION_TIERS: AIChatToolGovernancePermissionTier[] = [
  'basic',
  'advanced',
  'full',
];

function isToolGovernancePermissionTier(
  value: string
): value is AIChatToolGovernancePermissionTier {
  return value === 'basic' || value === 'advanced' || value === 'full';
}

function getToolGovernanceTierDisplay(
  tier: AIChatToolGovernancePermissionTier,
  t: ScopedTranslations<'webapp'>
) {
  if (tier === 'full') {
    return {
      Icon: ShieldAlert,
      label: t('consoleChat.governance.permissionTiers.full'),
      description: t('consoleChat.governance.permissionTiers.fullDescription'),
      warning: t('consoleChat.governance.permissionTiers.fullWarning'),
      iconClassName: 'text-destructive',
    };
  }

  if (tier === 'advanced') {
    return {
      Icon: ShieldCheck,
      label: t('consoleChat.governance.permissionTiers.advanced'),
      description: t('consoleChat.governance.permissionTiers.advancedDescription'),
      warning: '',
      iconClassName: 'text-primary',
    };
  }

  return {
    Icon: Shield,
    label: t('consoleChat.governance.permissionTiers.basic'),
    description: t('consoleChat.governance.permissionTiers.basicDescription'),
    warning: '',
    iconClassName: 'text-muted-foreground',
  };
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
  modelUseCase = 'agent',
  preferredModelUseCase,
  showMemoryToggle = true,
  showComposerExpandButton = false,
  isComposerExpanded = false,
  showToolGovernancePermissionControl = false,
  toolGovernancePermissionTier = 'basic',
  enableUpload = true,
  showFileLibraryPicker = true,
  showSkillManagement = false,
  skillManagementLabel,
  surface = 'aichat',
  onModelChange,
  onModelPropsChange,
  onUploadDocument,
  onUploadImage,
  onSelectFromFiles,
  onOpenSkillManagement,
  onMemoryEnabledChange,
  onToggleComposerExpanded,
  onToolGovernancePermissionTierChange,
  onSend,
  onStop,
}: AIChatInputToolbarProps) {
  const t = useT('webapp');
  const showStopButton = canStop ?? isSending;
  const activeGovernanceTier = getToolGovernanceTierDisplay(toolGovernancePermissionTier, t);
  const ActiveGovernanceIcon = activeGovernanceTier.Icon;

  return (
    <div className="flex items-center justify-between gap-2 px-1">
      <div className="flex min-w-0 items-center gap-1.5">
        {showModelSelector ? (
          <div className="w-44 min-w-0 sm:w-56">
            {isModelInitializing ? (
              <div className="h-8 rounded-full border border-border/70 bg-muted/50" />
            ) : (
              <ModelSelector
                modelType={modelUseCase}
                preferredUseCase={preferredModelUseCase}
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
        {showToolGovernancePermissionControl && onToolGovernancePermissionTierChange ? (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                className={cn(
                  'h-8 shrink-0 gap-1 rounded-full border border-border/70 bg-background px-2 text-xs font-medium text-muted-foreground hover:bg-muted/60 hover:text-foreground',
                  toolGovernancePermissionTier === 'full' &&
                    'border-destructive/30 bg-destructive/5 text-destructive hover:bg-destructive/10 hover:text-destructive'
                )}
                title={
                  toolGovernancePermissionTier === 'full'
                    ? activeGovernanceTier.warning
                    : activeGovernanceTier.description
                }
                aria-label={`${t('consoleChat.governance.permissionTiers.label')}: ${activeGovernanceTier.label}`}
              >
                <ActiveGovernanceIcon
                  className={cn('size-3.5', activeGovernanceTier.iconClassName)}
                />
                <span>{activeGovernanceTier.label}</span>
                <ChevronDown className="size-3 text-muted-foreground" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="w-72">
              <DropdownMenuLabel className="flex items-center gap-1.5 text-muted-foreground">
                <Shield className="size-3.5" />
                {t('consoleChat.governance.permissionTiers.label')}
              </DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuRadioGroup
                value={toolGovernancePermissionTier}
                onValueChange={value => {
                  if (isToolGovernancePermissionTier(value)) {
                    onToolGovernancePermissionTierChange(value);
                  }
                }}
              >
                {TOOL_GOVERNANCE_PERMISSION_TIERS.map(tier => {
                  const display = getToolGovernanceTierDisplay(tier, t);
                  const TierIcon = display.Icon;
                  return (
                    <DropdownMenuRadioItem
                      key={tier}
                      value={tier}
                      className={cn(
                        'items-start gap-2 py-2',
                        tier === 'full' && 'text-destructive focus:text-destructive'
                      )}
                    >
                      <TierIcon className={cn('mt-0.5 size-4', display.iconClassName)} />
                      <span className="min-w-0">
                        <span className="block text-sm font-medium">{display.label}</span>
                        <span
                          className={cn(
                            'mt-0.5 block text-xs font-normal leading-4 text-muted-foreground',
                            tier === 'full' && 'text-destructive/80'
                          )}
                        >
                          {tier === 'full' ? display.warning : display.description}
                        </span>
                      </span>
                    </DropdownMenuRadioItem>
                  );
                })}
              </DropdownMenuRadioGroup>
            </DropdownMenuContent>
          </DropdownMenu>
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
        {showSkillManagement && onOpenSkillManagement && skillManagementLabel ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                isIcon
                variant="ghost"
                className="size-8 rounded-full"
                onClick={onOpenSkillManagement}
                aria-label={skillManagementLabel}
              >
                <Settings2 className="size-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent side="top">{skillManagementLabel}</TooltipContent>
          </Tooltip>
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
