'use client';

import { useEffect, useMemo, useState } from 'react';
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
import type { PromptPickerSelection } from '@/services/types/prompt';

interface PromptPickerDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onApply: (selection: PromptPickerSelection) => void;
  applyMode?: 'reference' | 'copy';
  applyLabel?: string;
  warnOnReplace?: boolean;
}

export function PromptPickerDialog({
  open,
  onOpenChange,
  onApply,
  applyMode = 'reference',
  applyLabel,
  warnOnReplace = false,
}: PromptPickerDialogProps) {
  const t = useT('prompts');
  const currentWorkspace = useCurrentWorkspace();
  const [keyword, setKeyword] = useState('');
  const [selectedPromptId, setSelectedPromptId] = useState<string | null>(null);
  const [selectedVersionNumber, setSelectedVersionNumber] = useState<string>('');
  const [referenceMode, setReferenceMode] = useState<'label' | 'version'>('label');
  const [referenceLabel, setReferenceLabel] = useState<string>('production');
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
      prompt.versions.find(version => String(version.version) === selectedVersionNumber) ?? prompt.versions[0]
    );
  }, [prompt, selectedVersionNumber]);
  const availableLabels = useMemo(() => {
    const labels = new Set<string>();
    for (const version of prompt?.versions ?? []) {
      for (const label of version.labels) labels.add(label);
    }
    return Array.from(labels);
  }, [prompt?.versions]);
  const isCopyMode = applyMode === 'copy';

  const handleSelectPrompt = (promptId: string) => {
    setSelectedPromptId(promptId);
    setSelectedVersionNumber('');
  };

  const buildSelection = (): PromptPickerSelection | null => {
    if (!selectedVersion || !prompt) return null;
    return {
      prompt,
      version: selectedVersion,
      reference:
        referenceMode === 'label' && referenceLabel
          ? { mode: 'label', label: referenceLabel }
          : { mode: 'version', version: selectedVersion.version },
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
    if (!prompt) return;
    const labels = Array.from(
      new Set((prompt.versions || []).flatMap(version => version.labels))
    );
    if (labels.includes('production')) {
      setReferenceMode('label');
      setReferenceLabel('production');
      return;
    }
    if (labels.length > 0) {
      setReferenceMode('label');
      setReferenceLabel(labels[0] || '');
      return;
    }
    setReferenceMode('version');
  }, [prompt]);

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
              onChange={e => setKeyword(e.target.value)}
              placeholder={t('picker.searchPlaceholder')}
            />
            <div className="space-y-2 overflow-y-auto max-h-[360px]">
              {isLoading ? (
                <div className="text-sm text-muted-foreground">{t('states.loading')}</div>
              ) : prompts.length === 0 ? (
                <div className="text-sm text-muted-foreground">{t('states.empty')}</div>
              ) : (
                prompts.map(item => (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => handleSelectPrompt(item.id)}
                    className={`w-full text-left rounded-lg border p-3 transition-colors ${
                      selectedPromptId === item.id ? 'border-primary bg-primary/5' : 'hover:bg-muted/40'
                    }`}
                  >
                    <div className="flex items-center gap-2 flex-wrap">
                      <div className="font-medium">{item.name}</div>
                      <Badge variant="outline">{item.locale}</Badge>
                      <Badge variant="secondary">{t(`sources.${item.source}`)}</Badge>
                    </div>
                    {item.description ? (
                      <div className="text-xs text-muted-foreground mt-1 line-clamp-2">
                        {item.description}
                      </div>
                    ) : null}
                  </button>
                ))
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
                        {label}
                      </Badge>
                    ))}
                  </div>
                  <p className="text-sm text-muted-foreground">{prompt.description}</p>
                </div>
                <div
                  className={`grid grid-cols-1 gap-4 rounded-md border bg-muted/10 p-3 ${
                    isCopyMode ? '' : 'md:grid-cols-2'
                  }`}
                >
                  <div className="space-y-2">
                    <div className="text-xs font-medium text-muted-foreground">{t('picker.version')}</div>
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
                            v{version.version}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  {isCopyMode ? (
                    <div className="rounded-md border bg-background/80 px-3 py-3 text-sm text-muted-foreground">
                      {t('picker.copyModeDescription')}
                    </div>
                  ) : (
                    <>
                      <div className="space-y-2">
                        <div className="text-xs font-medium text-muted-foreground">{t('picker.referenceMode')}</div>
                        <Select
                          value={referenceMode}
                          onValueChange={value => setReferenceMode(value as 'label' | 'version')}
                        >
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {availableLabels.length > 0 ? (
                              <SelectItem value="label">{t('picker.followLabel')}</SelectItem>
                            ) : null}
                            <SelectItem value="version">{t('picker.pinVersion')}</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                      {referenceMode === 'label' && availableLabels.length > 0 ? (
                        <div className="space-y-2 md:col-span-2">
                          <div className="text-xs font-medium text-muted-foreground">{t('picker.label')}</div>
                          <Select value={referenceLabel} onValueChange={setReferenceLabel}>
                            <SelectTrigger>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              {availableLabels.map(label => (
                                <SelectItem key={label} value={label}>
                                  {label}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </div>
                      ) : null}
                    </>
                  )}
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
            {applyLabel || (isCopyMode ? t('picker.applyEditableCopy') : t('picker.apply'))}
          </Button>
        </DialogFooter>
      </DialogContent>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('picker.replaceWarningTitle')}
        description={
          isCopyMode
            ? t('picker.replaceWarningDescriptionCopy')
            : t('picker.replaceWarningDescriptionReference')
        }
        confirmText={t('picker.replaceWarningConfirm')}
        cancelText={t('actions.cancel')}
        onConfirm={handleConfirmApply}
        variant="warning"
      />
    </Dialog>
  );
}

export default PromptPickerDialog;
