import React, { useState, useEffect } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Switch } from '@/components/ui/switch';
import { Slider } from '@/components/ui/slider';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';
import { FileTypeSelector } from '../../../common/file-type-selector';
import { FileExtensionEditor } from '../../../common/file-extension-editor';
import { OptionEditor } from '@/components/ui/option-editor';
import type { InputVar, InputVarType } from '../../../types/input-var';
import { toast } from 'sonner';
import { InputVarType as InputVarTypeEnum, createDefaultInputVar } from '../../../types/input-var';
import { useT } from '@/i18n';
import { filterLowercaseExtensions, formatExtensionsForDisplay } from '@/utils/file-helpers';
import { sanitizeIdentifier } from '@/utils/validation';
import { useUploadConfig } from '@/hooks/use-upload';

interface VariableEditModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSave: (variable: InputVar) => void;
  variable?: InputVar | null;
}

const VariableEditModal: React.FC<VariableEditModalProps> = ({
  isOpen,
  onClose,
  onSave,
  variable,
}) => {
  const [localVariable, setLocalVariable] = useState<InputVar>(variable || createDefaultInputVar());
  const t = useT();
  const { data: uploadConfig } = useUploadConfig({ enabled: isOpen });
  const fileListMaxLength = Math.max(
    1,
    uploadConfig?.workflow_file_upload_limit ?? uploadConfig?.batch_count_limit ?? 10
  );

  useEffect(() => {
    if (isOpen) {
      const newVar = { ...(variable || createDefaultInputVar()) };
      if (newVar.type === InputVarTypeEnum.FILE || newVar.type === InputVarTypeEnum.FILE_LIST) {
        newVar.allowed_file_upload_methods = ['local_file'];
        newVar.allowed_file_extensions = formatExtensionsForDisplay(
          filterLowercaseExtensions(newVar.allowed_file_extensions ?? [])
        );
      }
      if (newVar.type === InputVarTypeEnum.FILE_LIST && newVar.max_length === undefined) {
        newVar.max_length = 5;
      }
      if (
        newVar.type === InputVarTypeEnum.FILE_LIST &&
        typeof newVar.max_length === 'number' &&
        newVar.max_length > fileListMaxLength
      ) {
        newVar.max_length = fileListMaxLength;
      }
      if (newVar.type === InputVarTypeEnum.TEXT_INPUT && newVar.max_length === undefined) {
        newVar.max_length = 20;
      }
      if (newVar.type === InputVarTypeEnum.PARAGRAPH && newVar.max_length === undefined) {
        newVar.max_length = 100;
      }
      if (!variable) {
        newVar.required = true;
      }
      setLocalVariable(newVar);
    }
  }, [fileListMaxLength, isOpen, variable]);

  const handleSave = () => {
    if (!localVariable.variable) {
      toast.error(t('nodes.start.modal.validation.title'), {
        description: t('nodes.start.modal.validation.variableNameRequired'),
      });
      return;
    }
    if (!localVariable.label) {
      toast.error(t('nodes.start.modal.validation.title'), {
        description: t('nodes.start.modal.validation.labelRequired'),
      });
      return;
    }

    const isFileVariable =
      localVariable.type === InputVarTypeEnum.FILE || localVariable.type === InputVarTypeEnum.FILE_LIST;

    let nextVariable: InputVar = { ...localVariable };

    if (isFileVariable) {
      const allowedTypes = nextVariable.allowed_file_types ?? [];
      const isCustomSelected = allowedTypes.includes('custom');
      const normalizedExtensions = filterLowercaseExtensions(
        nextVariable.allowed_file_extensions ?? []
      );

      if (allowedTypes.length === 0) {
        toast.error(t('nodes.start.modal.validation.title'), {
          description: t('nodes.start.modal.validation.fileTypeRequired'),
        });
        return;
      }

      if (isCustomSelected && normalizedExtensions.length === 0) {
        toast.error(t('nodes.start.modal.validation.title'), {
          description: t('nodes.start.modal.validation.customExtensionsRequired'),
        });
        return;
      }

      nextVariable = {
        ...nextVariable,
        allowed_file_upload_methods: ['local_file'],
        allowed_file_types: isCustomSelected
          ? ['custom']
          : allowedTypes.filter(type => type !== 'custom'),
        allowed_file_extensions: isCustomSelected ? normalizedExtensions : [],
      };
    }

    onSave(nextVariable);
    onClose();
  };

  const updateLocalVariable = (update: Partial<InputVar>) => {
    setLocalVariable(prev => ({ ...prev, ...update }));
  };

  const handleTypeChange = (type: string) => {
    const update: Partial<InputVar> = { type: type as InputVarType };
    // Reset default value based on new type
    if (type === InputVarTypeEnum.CHECKBOX) {
      update.default = false;
    } else if (type === InputVarTypeEnum.FILE || type === InputVarTypeEnum.FILE_LIST) {
      update.default = undefined;
      update.allowed_file_upload_methods = ['local_file'];
    } else {
      // For text-based types, convert to string or clear
      update.default = '';
    }
    // Set max_length based on type
    if (type === InputVarTypeEnum.FILE_LIST) {
      update.max_length = 5;
    } else if (type === InputVarTypeEnum.TEXT_INPUT) {
      update.max_length = 20;
    } else if (type === InputVarTypeEnum.PARAGRAPH) {
      update.max_length = 100;
    }
    updateLocalVariable(update);
  };

  const handleRequiredChange = (required: boolean) => {
    const update: Partial<InputVar> = { required };
    if (required) {
      update.hide = false;
    }
    updateLocalVariable(update);
  };

  const handleHideChange = (hide: boolean) => {
    const update: Partial<InputVar> = { hide };
    if (hide) {
      update.required = false;
    }
    updateLocalVariable(update);
  };

  const handleMaxLengthChange = (value: number[]) => {
    const numValue = value[0];
    if (localVariable.type === InputVarTypeEnum.FILE_LIST) {
      if (numValue > fileListMaxLength) {
        updateLocalVariable({ max_length: fileListMaxLength });
      } else if (numValue < 1) {
        updateLocalVariable({ max_length: 1 });
      } else {
        updateLocalVariable({ max_length: numValue });
      }
    } else {
      updateLocalVariable({
        max_length: numValue,
      });
    }
  };

  const getMaxLengthRange = (type: InputVarType): { min: number; max: number } => {
    switch (type) {
      case InputVarTypeEnum.TEXT_INPUT:
        return { min: 1, max: 200 };
      case InputVarTypeEnum.PARAGRAPH:
        return { min: 1, max: 2000 };
      case InputVarTypeEnum.FILE_LIST:
        return { min: 1, max: fileListMaxLength };
      default:
        return { min: 1, max: 1000 };
    }
  };

  return (
    <Dialog open={isOpen} onOpenChange={onClose}>
      <DialogContent className="max-w-[560px] p-0 overflow-hidden">
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-bold tracking-tight">
            {variable ? t('nodes.start.modal.title.edit') : t('nodes.start.modal.title.create')}
          </DialogTitle>
          <DialogDescription className="text-sm font-medium text-muted-foreground">
            {variable
              ? t('nodes.start.modal.description.edit')
              : t('nodes.start.modal.description.create')}
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-4 py-5 scrollbar-thin">
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="variable-name" className="text-sm font-bold px-0.5">
                  {t('nodes.start.modal.fields.variableName')}
                </Label>
                <Input
                  id="variable-name"
                  value={localVariable.variable}
                  className="h-10 shadow-sm font-medium"
                  onChange={e => {
                    const raw = e.target.value;
                    const cleaned = sanitizeIdentifier(raw);
                    updateLocalVariable({ variable: cleaned });
                  }}
                  maxLength={50}
                  placeholder={t('nodes.start.modal.placeholders.variableName')}
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="label" className="text-sm font-bold px-0.5">
                  {t('nodes.start.modal.fields.label')}
                </Label>
                <Input
                  id="label"
                  value={localVariable.label}
                  className="h-10 shadow-sm font-medium"
                  onChange={e => updateLocalVariable({ label: e.target.value })}
                  onFocus={() => {
                    if (!localVariable.label) {
                      updateLocalVariable({ label: localVariable.variable });
                    }
                  }}
                  maxLength={50}
                  placeholder={t('nodes.start.modal.placeholders.label')}
                />
              </div>
            </div>

            <div className="space-y-2">
              <Label className="text-sm font-bold px-0.5">
                {t('nodes.start.modal.fields.variableType')}
              </Label>
              <Select value={localVariable.type} onValueChange={handleTypeChange}>
                <SelectTrigger className="h-10 shadow-sm font-medium">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {Object.values(InputVarTypeEnum).map(type => (
                    <SelectItem key={type} value={type} className="font-medium">
                      {t(`nodes.start.types.${type}` as Parameters<typeof t>[0])}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Checkbox default value toggle */}
            {localVariable.type === InputVarTypeEnum.CHECKBOX && (
              <div className="flex items-center justify-between p-3 rounded-xl border border-neutral-100 bg-neutral-50/50 shadow-sm">
                <Label htmlFor="default-value" className="text-sm font-bold">
                  {t('nodes.start.modal.fields.defaultValue')}
                </Label>
                <Switch
                  id="default-value"
                  checked={localVariable.default === true}
                  onCheckedChange={checked => updateLocalVariable({ default: checked })}
                />
              </div>
            )}

            {/* Default value for text-based types (exclude FILE, FILE_LIST, CHECKBOX) */}
            {localVariable.type !== InputVarTypeEnum.CHECKBOX &&
              localVariable.type !== InputVarTypeEnum.FILE &&
              localVariable.type !== InputVarTypeEnum.FILE_LIST && (
                <div className="space-y-2">
                  <Label htmlFor="default-value" className="text-sm font-bold px-0.5">
                    {t('nodes.start.modal.fields.defaultValue')}
                  </Label>
                  {localVariable.type === InputVarTypeEnum.SELECT ? (
                    <Select
                      value={(() => {
                        const opts = (localVariable.options || [])
                          .map(o => String(o).trim())
                          .filter(o => o.length > 0);
                        const val = String((localVariable.default as string) || '').trim();
                        return opts.includes(val) ? val : '';
                      })()}
                      onValueChange={val => updateLocalVariable({ default: val })}
                    >
                      <SelectTrigger className="h-10 shadow-sm font-medium">
                        <SelectValue
                          placeholder={t('nodes.start.modal.placeholders.defaultValue')}
                        />
                      </SelectTrigger>
                      <SelectContent>
                        {(localVariable.options || [])
                          .map(opt => String(opt).trim())
                          .filter(opt => opt.length > 0)
                          .map((opt, idx) => (
                            <SelectItem key={`${opt}-${idx}`} value={opt} className="font-medium">
                              {opt}
                            </SelectItem>
                          ))}
                      </SelectContent>
                    </Select>
                  ) : localVariable.type === InputVarTypeEnum.PARAGRAPH ? (
                    <Textarea
                      id="default-value"
                      value={(localVariable.default as string) || ''}
                      className="max-h-32 shadow-sm font-medium resize-none bg-neutral-50/50 border-neutral-200"
                      onChange={e => updateLocalVariable({ default: e.target.value })}
                      placeholder={t('nodes.start.modal.placeholders.defaultValue')}
                      maxLength={
                        localVariable.max_length || getMaxLengthRange(localVariable.type).max
                      }
                      rows={3}
                    />
                  ) : (
                    <Input
                      id="default-value"
                      value={(localVariable.default as string) || ''}
                      className="h-10 shadow-sm font-medium bg-neutral-50/50 border-neutral-200"
                      onChange={e => updateLocalVariable({ default: e.target.value })}
                      placeholder={t('nodes.start.modal.placeholders.defaultValue')}
                      type={localVariable.type === InputVarTypeEnum.NUMBER ? 'number' : 'text'}
                      maxLength={
                        localVariable.type === InputVarTypeEnum.TEXT_INPUT
                          ? localVariable.max_length || getMaxLengthRange(localVariable.type).max
                          : undefined
                      }
                    />
                  )}
                </div>
              )}

            {(localVariable.type === InputVarTypeEnum.TEXT_INPUT ||
              localVariable.type === InputVarTypeEnum.PARAGRAPH ||
              localVariable.type === InputVarTypeEnum.FILE_LIST) && (
              <div className="space-y-4 pt-2">
                <Label
                  htmlFor="max-length"
                  className="text-sm font-bold px-0.5 flex items-center justify-between"
                >
                  <span>
                    {localVariable.type === InputVarTypeEnum.FILE_LIST
                      ? t('nodes.start.modal.fields.maxFiles', {
                          count: localVariable.max_length || 0,
                        })
                      : t('nodes.start.modal.fields.maxLength')}
                  </span>
                  {localVariable.type !== InputVarTypeEnum.FILE_LIST && (
                    <span className="text-xs text-muted-foreground font-bold bg-neutral-100 px-1.5 py-0.5 rounded-md">
                      {localVariable.max_length || 0}
                    </span>
                  )}
                </Label>
                {localVariable.type === InputVarTypeEnum.FILE_LIST ? (
                  <div className="px-1.5">
                    <Slider
                      id="max-length"
                      value={[localVariable.max_length || 5]}
                      onValueChange={handleMaxLengthChange}
                      min={getMaxLengthRange(localVariable.type).min}
                      max={getMaxLengthRange(localVariable.type).max}
                      step={1}
                      className="py-4"
                    />
                  </div>
                ) : (
                  <Input
                    id="max-length"
                    type="number"
                    className="h-10 shadow-sm font-medium"
                    value={localVariable.max_length || ''}
                    onChange={e =>
                      updateLocalVariable({
                        max_length: e.target.value
                          ? Math.max(
                              getMaxLengthRange(localVariable.type).min,
                              Math.min(
                                getMaxLengthRange(localVariable.type).max,
                                parseInt(e.target.value, 10)
                              )
                            )
                          : undefined,
                      })
                    }
                    min={getMaxLengthRange(localVariable.type).min}
                    max={getMaxLengthRange(localVariable.type).max}
                  />
                )}
              </div>
            )}

            {localVariable.type === InputVarTypeEnum.SELECT && (
              <div className="pt-2">
                <OptionEditor
                  options={localVariable.options || []}
                  onChange={options => updateLocalVariable({ options })}
                  labels={{
                    title: t('nodes.start.modal.select.optionsTitle'),
                    add: t('nodes.start.modal.select.addOption'),
                    placeholder: (index: number) =>
                      t('nodes.start.modal.select.optionN', { index }),
                  }}
                />
              </div>
            )}

            {(localVariable.type === InputVarTypeEnum.FILE ||
              localVariable.type === InputVarTypeEnum.FILE_LIST) && (
              <div className="space-y-4 pt-2">
                <div className="flex items-center gap-2">
                  <div className="h-4 w-1 bg-primary rounded-full" />
                  <Label className="text-sm font-bold uppercase tracking-wider text-primary/80">
                    {t('nodes.start.modal.file.title')}
                  </Label>
                </div>
                <div className="bg-neutral-50/50 p-4 rounded-2xl border border-neutral-100 space-y-4">
                  <FileTypeSelector
                    selectedTypes={localVariable.allowed_file_types || []}
                    onChange={types =>
                      updateLocalVariable({
                        allowed_file_types: types,
                        allowed_file_extensions: types.includes('custom')
                          ? localVariable.allowed_file_extensions
                          : [],
                      })
                    }
                    label={t('nodes.start.modal.file.allowedTypes')}
                  />
                  {localVariable.allowed_file_types?.includes('custom') && (
                    <div className="pt-2 border-t border-neutral-100/50">
                      <FileExtensionEditor
                        extensions={localVariable.allowed_file_extensions || []}
                        onChange={exts => updateLocalVariable({ allowed_file_extensions: exts })}
                        placeholder={t('nodes.start.modal.placeholders.allowedFileExtensions')}
                        label={t('nodes.start.modal.file.allowedExtensions')}
                        clearText={t('common.clear')}
                        addText={t('common.add')}
                      />
                    </div>
                  )}
                </div>
              </div>
            )}
            <div className="space-y-3">
              <div className="flex items-center justify-between p-3 rounded-xl border border-neutral-100 bg-neutral-50/30">
                <div className="space-y-0.5">
                  <Label htmlFor="required" className="text-sm font-bold">
                    {t('nodes.start.modal.toggles.required')}
                  </Label>
                </div>
                <Switch
                  id="required"
                  checked={localVariable.required}
                  onCheckedChange={handleRequiredChange}
                  disabled={localVariable.hide}
                />
              </div>

              <div className="flex items-center justify-between p-3 rounded-xl border border-neutral-100 bg-neutral-50/30">
                <div className="space-y-0.5">
                  <Label htmlFor="hide" className="text-sm font-bold">
                    {t('nodes.start.modal.toggles.hide')}
                  </Label>
                </div>
                <Switch
                  id="hide"
                  checked={localVariable.hide || false}
                  onCheckedChange={handleHideChange}
                  disabled={localVariable.required}
                />
              </div>
            </div>
          </div>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
          <Button variant="ghost" onClick={onClose} className="font-semibold">
            {t('common.cancel')}
          </Button>
          <Button onClick={handleSave} size="lg" className="px-10 font-bold shadow-sm">
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default VariableEditModal;
