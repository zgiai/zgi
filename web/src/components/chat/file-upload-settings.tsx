'use client';

import { useEffect, useRef, useState } from 'react';
import { SlidersHorizontal } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Slider } from '@/components/ui/slider';
import { Switch } from '@/components/ui/switch';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { AICHAT_DOCUMENT_EXTENSIONS } from '@/components/chat/variants/aichat/input-area-types';
import { FileExtensionEditor } from '@/components/workflow/common/file-extension-editor';
import { FileTypeSelector, type FileType } from '@/components/workflow/common/file-type-selector';
import { useT } from '@/i18n';
import type { ChatFileKind, FileUploadSelection } from '@/services/upload.service';

const DEFAULT_COUNT = 5;
const DEFAULT_SELECTION: FileUploadSelection = { allowed_file_types: ['document', 'image'] };
const CHAT_FILE_KINDS: readonly FileType[] = ['document', 'image', 'audio', 'video', 'custom'];

export function fileUploadSelectionEnabled(value?: FileUploadSelection | null) {
  return value == null || value.allowed_file_types.length > 0;
}

/**
 * File uploads in a chat, in the shape the workflow app uses: one row with a
 * switch and a settings button that opens the types, custom extensions, and
 * per-turn count. Used by Agents and digital humans.
 */
export function FileUploadSettings({
  enabled,
  value,
  onChangeEnabled,
  onChange,
  disabled = false,
}: {
  enabled: boolean;
  value?: FileUploadSelection | null;
  onChangeEnabled: (enabled: boolean) => void;
  onChange: (value: FileUploadSelection) => void;
  disabled?: boolean;
}) {
  const t = useT('webapp');
  const [open, setOpen] = useState(false);
  const selection = value && value.allowed_file_types.length > 0 ? value : DEFAULT_SELECTION;
  return (
    <>
      <div className="flex items-start gap-3 rounded-md border p-3">
        <div className="min-w-0 flex-1">
          <div className="text-sm font-medium">{t('consoleChat.uploadSettings.title')}</div>
          <div className="text-xs text-muted-foreground">{t('consoleChat.uploadSettings.description')}</div>
          {enabled ? (
            <div className="mt-1 truncate text-xs text-muted-foreground/80">{selectionSummary(selection, t)}</div>
          ) : null}
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <Tooltip disableHoverableContent>
            <TooltipTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                isIcon
                size="sm"
                interactive="subtle"
                disabled={disabled || !enabled}
                onClick={() => setOpen(true)}
                aria-label={t('consoleChat.uploadSettings.configure')}
              >
                <SlidersHorizontal className="size-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent side="top">{t('consoleChat.uploadSettings.configure')}</TooltipContent>
          </Tooltip>
          <Switch
            aria-label={t('consoleChat.uploadSettings.title')}
            checked={enabled}
            disabled={disabled}
            onCheckedChange={onChangeEnabled}
          />
        </div>
      </div>
      <FileUploadSettingsDialog
        open={open}
        onOpenChange={setOpen}
        value={selection}
        onSave={next => {
          onChange(next);
          setOpen(false);
        }}
      />
    </>
  );
}

function FileUploadSettingsDialog({
  open,
  onOpenChange,
  value,
  onSave,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  value: FileUploadSelection;
  onSave: (value: FileUploadSelection) => void;
}) {
  const t = useT('webapp');
  const tCommon = useT('common');
  const [types, setTypes] = useState<ChatFileKind[]>(value.allowed_file_types);
  const [extensions, setExtensions] = useState<string[]>([]);
  const [count, setCount] = useState(value.number_limits ?? DEFAULT_COUNT);
  useEffect(() => {
    if (!open) return;
    setTypes(value.allowed_file_types);
    setExtensions((value.allowed_file_extensions ?? []).map(extension => `.${extension}`));
    setCount(value.number_limits ?? DEFAULT_COUNT);
  }, [open, value]);
  const custom = types.includes('custom');
  const canSave = types.length > 0 && (!custom || extensions.length > 0);
  const stored = t('consoleChat.uploadSettings.storedNote');
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[560px]">
        <DialogHeader>
          <DialogTitle>{t('consoleChat.uploadSettings.dialogTitle')}</DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-6">
          <FileTypeSelector
            label={t('consoleChat.attachments.allowedTypes')}
            availableTypes={CHAT_FILE_KINDS}
            selectedTypes={types}
            onChange={next => setTypes(next as ChatFileKind[])}
            extensionsByType={{ document: AICHAT_DOCUMENT_EXTENSIONS }}
            notesByType={{
              document: t('consoleChat.uploadSettings.documentNote'),
              image: t('consoleChat.uploadSettings.imageNote'),
              audio: stored,
              video: stored,
              custom: stored,
            }}
            keepOne
          />
          {custom ? (
            <FileExtensionEditor
              extensions={extensions}
              onChange={setExtensions}
              label={t('consoleChat.uploadSettings.customExtensions')}
              placeholder={t('consoleChat.uploadSettings.customPlaceholder')}
              clearText={tCommon('clear')}
            />
          ) : null}
          <div className="space-y-3 border-t pt-4">
            <div className="flex items-center justify-between">
              <Label className="text-sm">{t('consoleChat.uploadSettings.count')}</Label>
              <span className="rounded-full bg-primary/10 px-3 py-0.5 text-sm font-medium text-primary">
                {t('consoleChat.uploadSettings.countValue', { count })}
              </span>
            </div>
            <Slider value={[count]} min={1} max={10} step={1} onValueChange={values => setCount(Math.max(1, values[0] ?? 1))} />
          </div>
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            {tCommon('cancel')}
          </Button>
          <Button
            type="button"
            disabled={!canSave}
            onClick={() =>
              onSave({
                allowed_file_types: types,
                ...(custom ? { allowed_file_extensions: extensions.map(extension => extension.replace(/^\./, '').toLowerCase()) } : {}),
                ...(count !== DEFAULT_COUNT ? { number_limits: count } : {}),
              })
            }
          >
            {tCommon('save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function selectionSummary(selection: FileUploadSelection, t: ReturnType<typeof useT<'webapp'>>) {
  const kinds = selection.allowed_file_types.includes('custom')
    ? (selection.allowed_file_extensions ?? []).map(extension => extension.toUpperCase()).join(', ')
    : selection.allowed_file_types.map(kind => t(`consoleChat.uploadSettings.kinds.${kind}`)).join(t('consoleChat.uploadSettings.separator'));
  return t('consoleChat.uploadSettings.summary', { kinds, count: selection.number_limits ?? DEFAULT_COUNT });
}

// DigitalHuman already persists an empty type list as upload-disabled. Keep
// that single source of truth instead of adding a second enablement field.
export function PolicyFileUploadSettings({
  value,
  onChange,
  disabled = false,
}: {
  value?: FileUploadSelection | null;
  onChange: (value: FileUploadSelection) => void;
  disabled?: boolean;
}) {
  const enabled = fileUploadSelectionEnabled(value);
  const lastEnabled = useRef<FileUploadSelection>(value && enabled ? value : DEFAULT_SELECTION);
  return (
    <FileUploadSettings
      enabled={enabled}
      value={value}
      disabled={disabled}
      onChange={next => {
        if (fileUploadSelectionEnabled(next)) lastEnabled.current = next;
        onChange(next);
      }}
      onChangeEnabled={next => {
        if (!next && enabled && value) lastEnabled.current = value;
        onChange(next ? lastEnabled.current : { allowed_file_types: [] });
      }}
    />
  );
}
