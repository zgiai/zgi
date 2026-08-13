'use client';

import { useState, useEffect } from 'react';
import { useT } from '@/i18n';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Loader2 } from 'lucide-react';

interface EditRoleInfoDialogProps {
  open: boolean;
  title: string;
  onOpenChange: (open: boolean) => void;
  initialName: string;
  initialDescription: string;
  onSave: (name: string, description: string) => Promise<void>;
  isLoading?: boolean;
}

const ROLE_NAME_MAX_LENGTH = 30;
const ROLE_DESCRIPTION_MAX_LENGTH = 200;

const getCharacterCount = (value: string) => Array.from(value).length;

export function EditRoleInfoDialog({
  open,
  title,
  onOpenChange,
  initialName,
  initialDescription,
  onSave,
  isLoading = false,
}: EditRoleInfoDialogProps) {
  const t = useT('dashboard');
  const [name, setName] = useState(initialName);
  const [description, setDescription] = useState(initialDescription);
  const [saving, setSaving] = useState(false);
  const [nameError, setNameError] = useState('');
  const [descriptionError, setDescriptionError] = useState('');

  // Update local state when initial values change
  useEffect(() => {
    if (open) {
      setName(initialName);
      setDescription(initialDescription);
      setNameError('');
      setDescriptionError('');
    }
  }, [open, initialName, initialDescription]);

  const handleSave = async () => {
    const trimmedName = name.trim();
    if (!trimmedName) {
      setNameError(t('organization.permissions.errors.nameRequired'));
      return;
    }

    if (getCharacterCount(trimmedName) > ROLE_NAME_MAX_LENGTH) {
      setNameError(t('organization.permissions.errors.nameTooLong'));
      return;
    }

    const trimmedDescription = description.trim();
    if (getCharacterCount(trimmedDescription) > ROLE_DESCRIPTION_MAX_LENGTH) {
      setDescriptionError(t('organization.permissions.errors.descriptionTooLong'));
      return;
    }

    setNameError('');
    setDescriptionError('');
    setSaving(true);
    try {
      await onSave(trimmedName, trimmedDescription);
      onOpenChange(false);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md p-0 overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle className="text-xl font-bold tracking-tight">{title}</DialogTitle>
        </DialogHeader>

        <DialogBody className="py-6 space-y-6">
          <div className="space-y-2">
            <Label htmlFor="edit-role-name" className="text-sm font-bold text-foreground ml-1">
              {t('organization.permissions.config.roleName')}
            </Label>
            <Input
              id="edit-role-name"
              value={name}
              onChange={e => {
                setName(e.target.value);
                if (nameError) setNameError('');
              }}
              placeholder={t('organization.permissions.config.roleNamePlaceholder')}
              errorText={nameError}
              disabled={isLoading || saving}
              className="h-12 rounded-xl border focus:border-brand-main focus:ring-brand-main/10 transition-all font-medium"
            />
            <p
              className={`text-right text-xs ${
                getCharacterCount(name) > ROLE_NAME_MAX_LENGTH
                  ? 'text-destructive'
                  : 'text-muted-foreground'
              }`}
            >
              {getCharacterCount(name)}/{ROLE_NAME_MAX_LENGTH}
            </p>
          </div>

          <div className="space-y-2">
            <Label
              htmlFor="edit-role-description"
              className="text-sm font-bold text-foreground ml-1"
            >
              {t('organization.permissions.config.roleDescription')}
            </Label>
            <Textarea
              id="edit-role-description"
              value={description}
              onChange={e => {
                setDescription(e.target.value);
                if (descriptionError) setDescriptionError('');
              }}
              placeholder={t('organization.permissions.config.roleDescriptionPlaceholder')}
              rows={4}
              aria-invalid={!!descriptionError}
              disabled={isLoading || saving}
              className="rounded-xl border focus:border-brand-main focus:ring-brand-main/10 transition-all resize-none font-medium p-4"
            />
            {descriptionError && <p className="text-sm text-destructive">{descriptionError}</p>}
            <p
              className={`text-right text-xs ${
                getCharacterCount(description) > ROLE_DESCRIPTION_MAX_LENGTH
                  ? 'text-destructive'
                  : 'text-muted-foreground'
              }`}
            >
              {getCharacterCount(description)}/{ROLE_DESCRIPTION_MAX_LENGTH}
            </p>
          </div>
        </DialogBody>

        <DialogFooter className="bg-muted/50 pt-4 pb-6 px-6 border-t gap-3">
          <Button
            variant="ghost"
            size="xl"
            onClick={() => onOpenChange(false)}
            disabled={saving}
            className="px-6 font-semibold"
          >
            {t('organization.permissions.config.cancel')}
          </Button>
          <Button
            size="xl"
            onClick={handleSave}
            disabled={saving || !name.trim()}
            className="px-8 font-semibold"
          >
            {saving && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
            {t('organization.permissions.config.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
