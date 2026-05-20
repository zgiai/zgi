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
import { Label } from '@/components/ui/label';
import { Input, PasswordInput } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import type { EnvironmentVariable } from '../../store/type';

function getIdentifierError(name: string) {
  const raw = (name || '').trim();
  if (raw.length === 0) {
    return 'agents.workflow.conversationVariables.validation.required';
  }
  if (!/^[A-Za-z]/.test(raw)) {
    return 'agents.workflow.conversationVariables.validation.mustStartWithLetter';
  }
  if (!/^[A-Za-z][A-Za-z0-9_]*$/.test(raw)) {
    return 'agents.workflow.conversationVariables.validation.allowedChars';
  }
  return '';
}

export interface EnvironmentVariableEditorDialogProps {
  open: boolean;
  editing: boolean;
  value: EnvironmentVariable;
  onChange: (next: EnvironmentVariable) => void;
  onOpenChange: (open: boolean) => void;
  onSubmit: (finalValue: EnvironmentVariable) => void;
  existingNames: string[];
  typeOptions: Array<EnvironmentVariable['type']>;
}

const EnvironmentVariableEditorDialog: React.FC<EnvironmentVariableEditorDialogProps> = ({
  open,
  editing,
  value,
  onChange,
  onOpenChange,
  onSubmit,
  existingNames,
  typeOptions,
}) => {
  const t = useT();
  const [nameError, setNameError] = React.useState<string>('');

  React.useEffect(() => {
    if (!open) setNameError('');
  }, [open]);

  const handleSave = () => {
    const raw = (value.name || '').trim();
    const reason = getIdentifierError(raw);
    if (reason) {
      setNameError(t(reason));
      return;
    }
    const dup = existingNames.includes(raw);
    if (dup) {
      setNameError(t('agents.workflow.conversationVariables.validation.duplicateName'));
      return;
    }
    onSubmit({ ...value, name: raw });
  };

  const valueEditable = !(value.type === 'secret' && value.is_secret_set);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="p-0 overflow-hidden max-w-[560px]">
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-bold tracking-tight">
            {editing
              ? t('agents.workflow.environmentVariables.dialog.editTitle')
              : t('agents.workflow.environmentVariables.dialog.addTitle')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="space-y-6 py-6">
          <div className="space-y-5">
            <div className="flex flex-col gap-2">
              <Label className="text-sm font-bold px-0.5">
                {t('agents.workflow.environmentVariables.fields.name')}
              </Label>
              <Input
                value={value.name}
                className="h-10 shadow-sm font-medium"
                onChange={e => {
                  const next = e.target.value;
                  onChange({ ...value, name: next });
                  const reason = getIdentifierError(next);
                  if (reason) {
                    setNameError(t(reason));
                    return;
                  }
                  const isDup = existingNames.includes(next.trim());
                  if (isDup)
                    setNameError(
                      t('agents.workflow.conversationVariables.validation.duplicateName')
                    );
                  else setNameError('');
                }}
                placeholder={t('agents.workflow.environmentVariables.fields.name')}
                errorText={nameError}
              />
            </div>

            <div className="flex flex-col gap-2">
              <Label className="text-sm font-bold px-0.5">
                {t('agents.workflow.environmentVariables.fields.type')}
              </Label>
              <Select
                value={value.type}
                onValueChange={val => {
                  const nextType = val as EnvironmentVariable['type'];
                  const clearValue = nextType === 'number' ? '' : '';
                  onChange({ ...value, type: nextType, value: clearValue });
                }}
              >
                <SelectTrigger className="h-10 shadow-sm font-medium">
                  <SelectValue
                    placeholder={t('agents.workflow.environmentVariables.fields.type')}
                  />
                </SelectTrigger>
                <SelectContent>
                  {typeOptions.map(t => (
                    <SelectItem key={t} value={t} className="font-medium">
                      {t}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="flex flex-col gap-2">
              <Label className="text-sm font-bold px-0.5">
                {t('agents.workflow.environmentVariables.fields.description')}
              </Label>
              <Textarea
                value={value.description ?? ''}
                onChange={e => onChange({ ...value, description: e.target.value })}
                placeholder={t('agents.workflow.environmentVariables.fields.description')}
                className="max-h-32 shadow-sm font-medium resize-none"
              />
            </div>

            <div className="flex flex-col gap-2 pt-2 border-t border-neutral-100">
              <Label className="text-sm font-bold px-0.5 flex items-center justify-between">
                {t('agents.workflow.environmentVariables.fields.value')}
                {value.type === 'secret' && value.is_secret_set && (
                  <Badge
                    variant="secondary"
                    className="text-[10px] font-bold h-5 px-1.5 uppercase tracking-wider"
                  >
                    {t('agents.workflow.environmentVariables.secretMasked')}
                  </Badge>
                )}
              </Label>

              <div className="relative group">
                {value.type === 'number' ? (
                  <Input
                    type="number"
                    value={value.value}
                    className="h-11 shadow-sm font-medium bg-neutral-50/50"
                    onChange={e => valueEditable && onChange({ ...value, value: e.target.value })}
                    placeholder={t(
                      'agents.workflow.environmentVariables.placeholders.defaultValue'
                    )}
                    disabled={!valueEditable}
                  />
                ) : value.type === 'secret' ? (
                  <PasswordInput
                    autoComplete="new-password"
                    className="h-11 shadow-sm font-medium bg-neutral-50/50"
                    value={value.is_secret_set ? '**********' : value.value}
                    onChange={e => valueEditable && onChange({ ...value, value: e.target.value })}
                    placeholder={t(
                      'agents.workflow.environmentVariables.placeholders.defaultValue'
                    )}
                    disabled={!valueEditable}
                  />
                ) : (
                  <Textarea
                    value={value.value}
                    className="max-h-40 min-h-[100px] shadow-sm font-medium bg-neutral-50/50"
                    onChange={e => valueEditable && onChange({ ...value, value: e.target.value })}
                    placeholder={t(
                      'agents.workflow.environmentVariables.placeholders.defaultValue'
                    )}
                    disabled={!valueEditable}
                  />
                )}
              </div>
            </div>
          </div>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
          <Button variant="ghost" onClick={() => onOpenChange(false)} className="font-semibold">
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

export default EnvironmentVariableEditorDialog;
