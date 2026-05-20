'use client';

import React from 'react';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import type { ParameterSchemaItem, ParameterType } from '../config';
import { isValidIdentifier } from '@/utils/validation';
import { Textarea } from '@/components/ui/textarea';
import { Label } from '@/components/ui/label';

export interface ParamEditorDialogProps {
  open: boolean;
  editing: boolean;
  value: ParameterSchemaItem;
  onChange: (next: ParameterSchemaItem) => void;
  onOpenChange: (open: boolean) => void;
  onSubmit: (finalValue: ParameterSchemaItem) => void;
  existingNames: string[];
  labels: {
    titleAdd: string;
    titleEdit: string;
    fieldName: string;
    fieldType: string;
    fieldDesc: string;
    required: string;
    actionCancel: string;
    actionSave: string;
    invalidIdentifier: string;
    duplicateName: string;
  };
  typeOptions: ParameterType[];
}

const ParamEditorDialog: React.FC<ParamEditorDialogProps> = props => {
  const {
    open,
    editing,
    value,
    onChange,
    onOpenChange,
    onSubmit,
    existingNames,
    labels,
    typeOptions,
  } = props;
  const [nameError, setNameError] = React.useState<string>('');

  React.useEffect(() => {
    if (!open) {
      setNameError('');
    }
  }, [open]);

  const handleSave = () => {
    const raw = (value.name || '').trim();
    if (!isValidIdentifier(raw)) {
      setNameError(labels.invalidIdentifier);
      return;
    }
    if (existingNames.includes(raw)) {
      setNameError(labels.duplicateName);
      return;
    }
    setNameError('');
    onSubmit({ ...value, name: raw });
  };

  const handleNameChange = (raw: string) => {
    // Auto-convert spaces to underscores
    const newVal = raw.replace(/\s+/g, '_');
    onChange({ ...value, name: newVal });

    // Real-time validation
    if (!isValidIdentifier(newVal)) {
      setNameError(labels.invalidIdentifier);
    } else if (existingNames.includes(newVal)) {
      setNameError(labels.duplicateName);
    } else {
      setNameError('');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[480px] p-0 overflow-hidden">
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-bold tracking-tight">
            {editing ? labels.titleEdit : labels.titleAdd}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="space-y-6 py-6 font-medium">
          <div className="space-y-5">
            <div className="flex flex-col gap-2">
              <Label className="text-sm font-bold px-0.5">{labels.fieldName}</Label>
              <Input
                value={value.name}
                className={cn(
                  'h-10 shadow-sm font-medium',
                  nameError
                    ? 'border-destructive focus-visible:ring-destructive bg-destructive/5'
                    : undefined
                )}
                onChange={e => handleNameChange(e.target.value)}
                placeholder={labels.fieldName}
              />
              {nameError ? (
                <div className="text-xs text-destructive font-bold px-1">{nameError}</div>
              ) : null}
            </div>

            <div className="flex flex-col gap-2">
              <Label className="text-sm font-bold px-0.5">{labels.fieldType}</Label>
              <Select
                value={value.type}
                onValueChange={val => onChange({ ...value, type: val as ParameterType })}
              >
                <SelectTrigger className="h-10 shadow-sm font-medium">
                  <SelectValue placeholder={labels.fieldType} />
                </SelectTrigger>
                <SelectContent>
                  {typeOptions.map(tp => (
                    <SelectItem key={tp} value={tp} className="font-medium">
                      {tp}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="flex flex-col gap-2">
              <Label className="text-sm font-bold px-0.5">{labels.fieldDesc}</Label>
              <Textarea
                value={value.description || ''}
                onChange={e => onChange({ ...value, description: e.target.value })}
                placeholder={labels.fieldDesc}
                className="max-h-32 shadow-sm font-medium resize-none"
              />
            </div>

            <div className="flex items-center justify-between p-3 rounded-xl border border-neutral-100 bg-neutral-50/50 shadow-sm">
              <Label className="text-sm font-bold">{labels.required}</Label>
              <Switch
                checked={Boolean(value.required)}
                onCheckedChange={val => onChange({ ...value, required: val })}
              />
            </div>
          </div>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
          <Button variant="ghost" onClick={() => onOpenChange(false)} className="font-semibold">
            {labels.actionCancel}
          </Button>
          <Button onClick={handleSave} size="lg" className="px-10 font-bold shadow-sm">
            {labels.actionSave}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default ParamEditorDialog;
