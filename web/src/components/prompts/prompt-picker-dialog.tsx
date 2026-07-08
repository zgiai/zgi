'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { usePrompt, usePrompts } from '@/hooks/prompt/use-prompts';
import { useCurrentWorkspace } from '@/store/workspace-store';
import { promptLocaleLabelKey } from '@/components/prompts/prompt-display-labels';
import type { PromptPickerSelection } from '@/services/types/prompt';

interface PromptPickerDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onApply: (selection: PromptPickerSelection) => void;
  applyLabel?: string;
  warnOnReplace?: boolean;
}

export function PromptPickerDialog({
  open,
  onOpenChange,
  onApply,
  applyLabel,
  warnOnReplace = false,
}: PromptPickerDialogProps) {
  const t = useT('prompts');
  const currentWorkspace = useCurrentWorkspace();
  const [keyword, setKeyword] = useState('');
  const [selectedPromptId, setSelectedPromptId] = useState<string | null>(null);
  const [selectedVersionNumber, setSelectedVersionNumber] = useState<string>('');
  const [confirmOpen, setConfirmOpen] = useState(false);

  const { prompts, isLoading } = usePrompts(
    {
      keyword: keyword || undefined,
      workspace_id: currentWorkspace?.id,
      limit: 50,
    },
    open
  );
  const { prompt } = usePrompt(selectedPromptId || undefined, open && !!selectedPromptId);
  const selectedVersion = useMemo(() => {
    if (!prompt?.versions?.length) return undefined;
    if (!selectedVersionNumber) return prompt.versions[0];
    return (
      prompt.versions.find(version => String(version.version) === selectedVersionNumber) ??
      prompt.versions[0]
    );
  }, [prompt, selectedVersionNumber]);
  const displayReferenceLabel = useCallback(
    (label: string) => {
      const normalized = label.toLowerCase();
      if (normalized === 'production') return t('picker.releaseLabels.production');
      if (normalized === 'latest') return t('picker.releaseLabels.latest');
      if (normalized === 'staging') return t('picker.releaseLabels.staging');
      if (normalized === 'gray-a') return t('picker.releaseLabels.grayA');
      if (normalized === 'gray-b') return t('picker.releaseLabels.grayB');
      return label;
    },
    [t]
  );
  const primaryActionLabel = applyLabel || t('picker.applyEditableCopy');

  const clearSelection = useCallback(() => {
    setSelectedPromptId(null);
    setSelectedVersionNumber('');
  }, []);

  const handleSelectPrompt = (promptId: string) => {
    setSelectedPromptId(promptId);
    setSelectedVersionNumber('');
  };

  const handleKeywordChange = (value: string) => {
    setKeyword(value);
    if (selectedPromptId) {
      clearSelection();
    }
  };

  const buildSelection = (): PromptPickerSelection | null => {
    if (!selectedVersion || !prompt) return null;
    return {
      prompt,
      version: selectedVersion,
    };
  };

  const handleApply = () => {
    const selection = buildSelection();
    if (!selection) return;
    if (warnOnReplace) {
      setConfirmOpen(true);
      return;
    }
    onApply(selection);
    onOpenChange(false);
  };

  const handleConfirmApply = () => {
    const selection = buildSelection();
    if (!selection) return;
    onApply(selection);
    onOpenChange(false);
  };

  useEffect(() => {
    if (!open) return;
    setKeyword('');
    clearSelection();
    setConfirmOpen(false);
  }, [clearSelection, open]);

  useEffect(() => {
    if (!open || isLoading || !selectedPromptId) return;
    if (!prompts.some(item => item.id === selectedPromptId)) {
      clearSelection();
    }
  }, [clearSelection, isLoading, open, prompts, selectedPromptId]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-5xl">
        <DialogHeader>
          <DialogTitle>{t('picker.title')}</DialogTitle>
        </DialogHeader>
        <DialogBody className="grid grid-cols-1 lg:grid-cols-[280px_minmax(0,1fr)] gap-4 min-h-[460px]">
          <div className="space-y-3 border rounded-lg p-3 overflow-hidden">
            <Input
              value={keyword}
              onChange={e => handleKeywordChange(e.target.value)}
              placeholder={t('picker.searchPlaceholder')}
            />
            <div className="space-y-2 overflow-y-auto max-h-[360px]">
              {isLoading ? (
                <div className="text-sm text-muted-foreground">{t('states.loading')}</div>
              ) : prompts.length === 0 ? (
                <div className="text-sm text-muted-foreground">{t('states.empty')}</div>
              ) : (
                prompts.map(item => {
                  const hasSingleVersion = item.latest_version <= 1;

                  return (
                    <button
                      key={item.id}
                      type="button"
                      onClick={() => handleSelectPrompt(item.id)}
                      className={`w-full text-left rounded-lg border p-3 transition-colors ${
                        selectedPromptId === item.id
                          ? 'border-primary bg-primary/5'
                          : 'hover:bg-muted/40'
                      }`}
                    >
                      <div className="flex items-center gap-2 flex-wrap">
                        <div className="font-medium">{item.name}</div>
                        <Badge variant="outline">{t(promptLocaleLabelKey(item.locale))}</Badge>
                        <Badge variant="secondary">{t(`sources.${item.source}`)}</Badge>
                      </div>
                      <div className="mt-2 flex flex-wrap gap-1.5">
                        {hasSingleVersion ? (
                          <Badge variant="subtle">
                            {t('library.currentVersion', { version: item.latest_version })}
                          </Badge>
                        ) : (
                          <>
                            <Badge variant="subtle">
                              {t('picker.latestVersionShort', {
                                version: `v${item.latest_version}`,
                              })}
                            </Badge>
                            <Badge variant={item.production_version ? 'default' : 'warning'}>
                              {item.production_version
                                ? t('picker.onlineVersionShort', {
                                    version: `v${item.production_version}`,
                                  })
                                : t('picker.onlineVersionUnsetShort')}
                            </Badge>
                          </>
                        )}
                      </div>
                      {item.description ? (
                        <div className="text-xs text-muted-foreground mt-1 line-clamp-2">
                          {item.description}
                        </div>
                      ) : null}
                    </button>
                  );
                })
              )}
            </div>
          </div>
          <div className="border rounded-lg p-4 overflow-y-auto">
            {!prompt || !selectedVersion ? (
              <div className="text-sm text-muted-foreground">{t('picker.previewPlaceholder')}</div>
            ) : (
              <div className="space-y-4">
                <div className="space-y-2">
                  <div className="flex items-center gap-2 flex-wrap">
                    <h3 className="text-lg font-semibold">{prompt.name}</h3>
                    <Badge variant="outline">v{selectedVersion.version}</Badge>
                    {selectedVersion.labels.map(label => (
                      <Badge key={label} variant={label === 'production' ? 'default' : 'secondary'}>
                        {displayReferenceLabel(label)}
                      </Badge>
                    ))}
                  </div>
                  <p className="text-sm text-muted-foreground">{prompt.description}</p>
                </div>
                <div className="grid grid-cols-1 gap-4 rounded-md border bg-muted/10 p-3">
                  <div className="space-y-2">
                    <div className="text-xs font-medium text-muted-foreground">
                      {t('picker.version')}
                    </div>
                    <Select
                      value={String(selectedVersion.version)}
                      onValueChange={value => setSelectedVersionNumber(value)}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {(prompt.versions || []).map(version => (
                          <SelectItem key={version.id} value={String(version.version)}>
                            {[
                              `v${version.version}`,
                              ...version.labels.map(label => displayReferenceLabel(label)),
                            ].join(' · ')}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="rounded-md border bg-background/80 px-3 py-3 text-sm text-muted-foreground">
                    {t('picker.copyModeDescription')}
                  </div>
                </div>
                <div className="rounded-md border bg-muted/20 p-3">
                  <pre className="text-xs whitespace-pre-wrap break-words">
                    {typeof selectedVersion.content === 'string'
                      ? selectedVersion.content
                      : JSON.stringify(selectedVersion.content, null, 2)}
                  </pre>
                </div>
              </div>
            )}
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
          <Button onClick={handleApply} disabled={!selectedVersion}>
            {primaryActionLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('picker.replaceWarningTitle')}
        description={
          <span className="block space-y-1.5">
            <span className="block">{t('picker.replaceWarningDescriptionCopy')}</span>
            <span className="block text-xs leading-5 text-muted-foreground">
              {t('picker.replaceWarningSaveHintCopy')}
            </span>
          </span>
        }
        confirmText={t('picker.replaceWarningConfirm')}
        cancelText={t('actions.cancel')}
        onConfirm={handleConfirmApply}
        contentClassName="max-w-[380px] rounded-lg"
        footerClassName="border-t-0 bg-transparent px-5 pb-5 pt-2"
        titleClassName="text-base font-semibold leading-6"
        descriptionClassName="text-sm leading-6 font-normal"
        cancelClassName="!h-[34px] !rounded !px-4 !text-[13px]"
        confirmClassName="!h-[34px] !rounded !px-4 !text-[13px]"
      />
    </Dialog>
  );
}

export default PromptPickerDialog;
