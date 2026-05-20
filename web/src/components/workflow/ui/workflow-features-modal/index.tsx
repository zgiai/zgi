'use client';

import { useEffect, useMemo, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Checkbox } from '@/components/ui/checkbox';
import { Badge } from '@/components/ui/badge';
import { Slider } from '@/components/ui/slider';
import { cn } from '@/lib/utils';
import type { WorkflowFeatures } from '@/components/workflow/store/type';
import { useWorkflowStore } from '@/components/workflow/store';
import { useT } from '@/i18n';

export type ChatFeatures = Pick<WorkflowFeatures, 'retriever_resource' | 'file_upload'>;

interface WorkflowFeaturesModalProps {
  open: boolean;
  onClose: () => void;
}

// Using shared extension constants from utils for consistency

export default function WorkflowFeaturesModal({ open, onClose }: WorkflowFeaturesModalProps) {
  const t = useT();
  const storeWorkflowData = useWorkflowStore.use.workflowData();
  const updateWorkflowFeatures = useWorkflowStore.use.updateWorkflowFeatures();
  const [form, setForm] = useState<ChatFeatures>({
    retriever_resource: storeWorkflowData?.features?.retriever_resource,
    file_upload: storeWorkflowData?.features?.file_upload,
  });
  const [customExtInput, setCustomExtInput] = useState('');

  useEffect(() => {
    if (open) {
      setForm({
        retriever_resource: storeWorkflowData?.features?.retriever_resource,
        file_upload: storeWorkflowData?.features?.file_upload,
      });
      setCustomExtInput('');
    }
  }, [open, storeWorkflowData.features]);

  const isCustomSelected = useMemo(
    () => form.file_upload?.allowed_file_types?.includes('custom') ?? false,
    [form.file_upload]
  );

  const handleToggleRetriever = (enabled: boolean) => {
    setForm(prev => ({ ...prev, retriever_resource: { enabled } }));
  };

  const handleToggleUpload = (enabled: boolean) => {
    setForm(prev => ({
      ...prev,
      file_upload: {
        ...prev.file_upload,
        enabled,
      },
    }));
  };

  const toggleAllowedType = (
    type: 'image' | 'document' | 'custom' | 'audio' | 'video',
    checked: boolean
  ) => {
    setForm(prev => {
      const current = new Set(prev.file_upload?.allowed_file_types ?? []);
      if (checked) {
        current.add(type);
        // Ensure 'custom' is exclusive
        if (type === 'custom') {
          current.clear();
          current.add('custom');
        } else {
          current.delete('custom');
        }
      } else {
        current.delete(type);
      }
      return {
        ...prev,
        file_upload: {
          ...prev.file_upload,
          allowed_file_types: Array.from(current),
        },
      };
    });
  };

  const addCustomExtension = () => {
    const raw = customExtInput.trim();
    if (!raw) return;
    const cleaned = raw.toLowerCase().replace(/^\./, '');
    setForm(prev => ({
      ...prev,
      file_upload: {
        ...prev.file_upload,
        allowed_file_extensions: Array.from(
          new Set([...(prev.file_upload?.allowed_file_extensions ?? []), cleaned])
        ),
      },
    }));
    setCustomExtInput('');
  };

  const removeCustomExtension = (ext: string) => {
    setForm(prev => ({
      ...prev,
      file_upload: {
        ...prev.file_upload,
        allowed_file_extensions: (prev.file_upload?.allowed_file_extensions ?? []).filter(
          e => e !== ext
        ),
      },
    }));
  };

  const setNumberLimit = (valueNum: number) => {
    setForm(prev => ({
      ...prev,
      file_upload: {
        ...prev.file_upload,
        number_limits: valueNum,
      },
    }));
  };

  const canSave = useMemo(() => {
    // Basic validation: if file_upload enabled, must have at least one allowed type
    if (!form.file_upload?.enabled) return true;
    const types = form.file_upload.allowed_file_types ?? [];
    if (types.length === 0) return false;
    // If custom selected, require at least one extension
    if (types.includes('custom') && (form.file_upload.allowed_file_extensions?.length ?? 0) === 0) {
      return false;
    }
    const numLimit = form.file_upload.number_limits ?? 0;
    return numLimit > 0;
  }, [form.file_upload]);

  const handleSave = () => {
    if (!canSave) return;
    // Normalize extensions to lowercase without leading dots
    const normalizedExt = (form.file_upload?.allowed_file_extensions ?? []).map(e =>
      e.toLowerCase().replace(/^\./, '')
    );
    updateWorkflowFeatures({
      retriever_resource: form.retriever_resource,
      file_upload: form.file_upload
        ? {
            ...form.file_upload,
            allowed_file_extensions: normalizedExt,
            allowed_file_upload_methods: form.file_upload.allowed_file_upload_methods ?? [
              'local_file',
              'remote_url',
            ],
          }
        : form.file_upload,
    });
    onClose();
  };

  return (
    <Dialog open={open} onOpenChange={v => !v && onClose()}>
      <DialogContent className="max-w-[600px] p-0 overflow-hidden">
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-bold tracking-tight">
            {t('agents.workflow.features.title')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="space-y-6 py-6">
          {/* Retriever Feature Card */}
          <div className="group relative flex items-center justify-between rounded-2xl border border-neutral-200 bg-white p-5 shadow-premium transition-all hover:ring-1 hover:ring-primary/10">
            <div className="space-y-1.5 flex-1 pr-6">
              <Label className="text-base font-bold tracking-tight">
                {t('agents.workflow.features.retrieverLabel')}
              </Label>
              <p className="text-sm text-muted-foreground font-medium leading-relaxed">
                {t('agents.workflow.features.retrieverDesc')}
              </p>
            </div>
            <Switch
              checked={form.retriever_resource?.enabled ?? false}
              onCheckedChange={handleToggleRetriever}
              className="data-[state=checked]:bg-primary"
            />
          </div>

          {/* Upload Feature Card */}
          <div className="group relative rounded-2xl border border-neutral-200 bg-white shadow-premium transition-all overflow-hidden border-l-4 border-l-primary/10">
            <div className="flex items-center justify-between p-5 border-b border-neutral-100">
              <div className="space-y-1.5 flex-1 pr-6">
                <Label className="text-base font-bold tracking-tight">
                  {t('agents.workflow.features.uploadLabel')}
                </Label>
                <p className="text-sm text-muted-foreground font-medium leading-relaxed">
                  {t('agents.workflow.features.uploadDesc')}
                </p>
              </div>
              <Switch
                checked={form.file_upload?.enabled ?? false}
                onCheckedChange={handleToggleUpload}
                className="data-[state=checked]:bg-primary"
              />
            </div>

            {form.file_upload?.enabled && (
              <div className="p-5 space-y-8 bg-neutral-50/30">
                <div className="space-y-4">
                  <Label className="text-xs uppercase tracking-widest text-muted-foreground font-bold">
                    {t('agents.workflow.features.allowedTypes')}
                  </Label>
                  <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
                    {[
                      { id: 'type-image', type: 'image' },
                      { id: 'type-audio', type: 'audio' },
                      { id: 'type-video', type: 'video' },
                      { id: 'type-document', type: 'document' },
                      { id: 'type-custom', type: 'custom' },
                    ].map(item => (
                      <div
                        key={item.id}
                        className={cn(
                          'flex items-center gap-2.5 p-3 rounded-xl border transition-all cursor-pointer',
                          (form.file_upload?.allowed_file_types ?? []).includes(item.type as any)
                            ? 'border-primary bg-primary/[0.03] shadow-sm'
                            : 'border-neutral-200 bg-white hover:border-neutral-300'
                        )}
                        onClick={() =>
                          toggleAllowedType(
                            item.type as any,
                            !(form.file_upload?.allowed_file_types ?? []).includes(item.type as any)
                          )
                        }
                      >
                        <Checkbox
                          id={item.id}
                          checked={(form.file_upload?.allowed_file_types ?? []).includes(
                            item.type as any
                          )}
                          onCheckedChange={c => toggleAllowedType(item.type as any, Boolean(c))}
                          className="data-[state=checked]:bg-primary border-neutral-300"
                        />
                        <Label
                          htmlFor={item.id}
                          className="text-sm font-semibold cursor-pointer select-none"
                        >
                          {t(`agents.workflow.features.typeLabels.${item.type}` as any)}
                        </Label>
                      </div>
                    ))}
                  </div>

                  {isCustomSelected && (
                    <div className="space-y-3 pt-2">
                      <Label className="text-[11px] uppercase tracking-wider text-muted-foreground font-bold px-1">
                        {t('agents.workflow.features.allowedExtensions')}
                      </Label>
                      <div className="flex gap-2">
                        <Input
                          value={customExtInput}
                          className="h-10 shadow-sm"
                          placeholder={t('agents.workflow.features.customPlaceholder')}
                          onChange={e => setCustomExtInput(e.target.value)}
                          onKeyDown={e =>
                            e.key === 'Enter' && (e.preventDefault(), addCustomExtension())
                          }
                        />
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={addCustomExtension}
                          className="font-bold border-neutral-300"
                        >
                          {t('common.add')}
                        </Button>
                      </div>
                      <div className="flex flex-wrap gap-2 pt-1">
                        {(form.file_upload?.allowed_file_extensions ?? []).map(ext => (
                          <Badge
                            key={ext}
                            variant="secondary"
                            className="h-7 pl-3 pr-1.5 flex items-center gap-1.5 bg-white border border-neutral-200 text-xs font-bold"
                          >
                            .{ext}
                            <button
                              type="button"
                              className="size-4 flex items-center justify-center rounded-full hover:bg-neutral-100 transition-colors"
                              onClick={e => {
                                e.stopPropagation();
                                removeCustomExtension(ext);
                              }}
                            >
                              ✕
                            </button>
                          </Badge>
                        ))}
                      </div>
                    </div>
                  )}
                </div>

                <div className="space-y-4 pt-4 border-t border-neutral-100">
                  <div className="flex items-center justify-between">
                    <Label className="text-sm font-bold">
                      {t('agents.workflow.features.totalFileCount')}
                    </Label>
                    <span className="text-sm font-bold bg-primary/10 text-primary px-2.5 py-0.5 rounded-full">
                      {form.file_upload?.number_limits ?? 1}
                    </span>
                  </div>
                  <div className="px-2">
                    <Slider
                      value={[form.file_upload?.number_limits ?? 1]}
                      min={1}
                      max={10}
                      step={1}
                      onValueChange={(vals: number[]) => setNumberLimit(Math.max(1, vals[0] ?? 1))}
                    />
                  </div>
                </div>
              </div>
            )}
          </div>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
          <Button type="button" variant="ghost" onClick={onClose} className="font-semibold">
            {t('common.cancel')}
          </Button>
          <Button
            type="button"
            onClick={handleSave}
            disabled={!canSave}
            size="lg"
            className="px-10 font-bold"
          >
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
