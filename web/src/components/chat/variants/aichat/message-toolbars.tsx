'use client';

import { Check, ChevronLeft, ChevronRight, Copy, Pencil, RotateCcw, X } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import type { ChatBranchNavigation } from '@/components/chat/utils/message-tree';

interface UserMessageToolbarProps {
  query: string;
  canEdit: boolean;
  isDisabled: boolean;
  toolbarVisibility: string;
  onEdit: () => void;
}

interface UserEditToolbarProps {
  canSubmit: boolean;
  isSending: boolean;
  onCancel?: () => void;
  onSubmit: () => void;
}

interface AssistantMessageToolbarProps {
  answer: string;
  canRegenerate: boolean;
  isDisabled: boolean;
  toolbarVisibility: string;
  branchNavigation?: ChatBranchNavigation;
  canSwitchBranch: boolean;
  onRegenerate?: () => void;
  onSwitchBranch?: (messageId: string) => void;
}

interface CopyActionButtonProps {
  text: string;
  title: string;
}

function CopyActionButton({ text, title }: CopyActionButtonProps) {
  const commonT = useT('common');
  const [copied, setCopied] = useState(false);
  const resetTimerRef = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (resetTimerRef.current !== null) {
        window.clearTimeout(resetTimerRef.current);
      }
    };
  }, []);

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      toast.success(commonT('toasts.copySuccess'));
      if (resetTimerRef.current !== null) {
        window.clearTimeout(resetTimerRef.current);
      }
      resetTimerRef.current = window.setTimeout(() => {
        setCopied(false);
        resetTimerRef.current = null;
      }, 1200);
    } catch {
      toast.error(commonT('toasts.copyFailed'));
    }
  }, [commonT, text]);

  return (
    <Button
      variant="ghost"
      isIcon
      size="xs"
      className="size-6 text-muted-foreground"
      onClick={handleCopy}
      title={title}
    >
      {copied ? (
        <Check className="size-3.5 text-success" />
      ) : (
        <Copy className="size-3.5" />
      )}
    </Button>
  );
}

/**
 * @component UserMessageToolbar
 * @category Feature
 * @status Stable
 * @description Hover toolbar for AIChat user message actions.
 * @usage Render below a persisted user query bubble
 * @example
 * <UserMessageToolbar query={query} canEdit onEdit={openEdit} />
 */
export function UserMessageToolbar({
  query,
  canEdit,
  isDisabled,
  toolbarVisibility,
  onEdit,
}: UserMessageToolbarProps) {
  const t = useT('webapp');

  if (!canEdit && !query) return null;

  return (
    <div
      className={cn(
        'mt-1 flex items-center justify-end gap-1 transition-opacity',
        toolbarVisibility
      )}
    >
      <CopyActionButton text={query} title={t('chat.copy')} />
      {canEdit ? (
        <Button
          variant="ghost"
          isIcon
          size="xs"
          className="size-6 text-muted-foreground"
          disabled={isDisabled}
          onClick={onEdit}
          title={t('consoleChat.editMessage')}
        >
          <Pencil className="size-3.5" />
        </Button>
      ) : null}
    </div>
  );
}

/**
 * @component UserEditToolbar
 * @category Feature
 * @status Stable
 * @description Inline edit controls for an AIChat user message.
 * @usage Render under the edit textarea
 * @example
 * <UserEditToolbar canSubmit onSubmit={sendEdit} />
 */
export function UserEditToolbar({
  canSubmit,
  isSending,
  onCancel,
  onSubmit,
}: UserEditToolbarProps) {
  const t = useT('webapp');
  const commonT = useT('common');

  return (
    <div className="flex justify-end gap-1">
      <Button
        variant="ghost"
        isIcon
        size="sm"
        className="size-7 text-muted-foreground"
        onClick={onCancel}
        title={commonT('cancel')}
      >
        <X size={18} />
      </Button>
      <Button
        isIcon
        size="sm"
        className="size-7"
        disabled={!canSubmit || isSending}
        onClick={onSubmit}
        title={t('consoleChat.sendEdited')}
      >
        <Check size={18} />
      </Button>
    </div>
  );
}

/**
 * @component AssistantMessageToolbar
 * @category Feature
 * @status Stable
 * @description Answer toolbar with branch navigation and assistant actions.
 * @usage Render below a persisted assistant answer
 * @example
 * <AssistantMessageToolbar answer={answer} canRegenerate />
 */
export function AssistantMessageToolbar({
  answer,
  canRegenerate,
  isDisabled,
  toolbarVisibility,
  branchNavigation,
  canSwitchBranch,
  onRegenerate,
  onSwitchBranch,
}: AssistantMessageToolbarProps) {
  const t = useT('webapp');

  return (
    <div className="mt-2 flex items-center gap-1">
      {branchNavigation ? (
        <div className="flex items-center gap-0.5 text-muted-foreground">
          <Button
            variant="ghost"
            size="xs"
            className="size-6 text-muted-foreground"
            disabled={!canSwitchBranch}
            onClick={() => onSwitchBranch?.(branchNavigation.previousId)}
            title={t('consoleChat.previousBranch')}
          >
            <ChevronLeft size={18} />
          </Button>
          <span className="min-w-9 text-center text-[13px] text-foreground">
            {branchNavigation.current} / {branchNavigation.total}
          </span>
          <Button
            variant="ghost"
            isIcon
            size="xs"
            className="size-6 text-muted-foreground"
            disabled={!canSwitchBranch}
            onClick={() => onSwitchBranch?.(branchNavigation.nextId)}
            title={t('consoleChat.nextBranch')}
          >
            <ChevronRight size={18} />
          </Button>
        </div>
      ) : null}
      <div className={cn('flex items-center gap-1 transition-opacity', toolbarVisibility)}>
        {answer ? (
          <CopyActionButton text={answer} title={t('chat.copy')} />
        ) : null}
        {canRegenerate ? (
          <Button
            variant="ghost"
            isIcon
            size="xs"
            className="size-6 text-muted-foreground"
            disabled={isDisabled}
            onClick={onRegenerate}
            title={t('chat.regenerate')}
          >
            <RotateCcw className="size-3.5" />
          </Button>
        ) : null}
      </div>
    </div>
  );
}
