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

  // Update local state when initial values change
  useEffect(() => {
    if (open) {
      setName(initialName);
      setDescription(initialDescription);
      setNameError('');
    }
  }, [open, initialName, initialDescription]);

  const handleSave = async () => {
    const trimmedName = name.trim();
    if (!trimmedName) {
      setNameError(t('organization.validation.nameRequired'));
      return;
    }

    if (trimmedName.length > 30) {
      setNameError(t('organization.validation.nameTooLong', { max: 30 }));
      return;
    }

    setSaving(true);
    try {
      await onSave(trimmedName, description.trim());
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
              maxLength={30}
              errorText={nameError}
              disabled={isLoading || saving}
              className="h-12 rounded-xl border focus:border-brand-main focus:ring-brand-main/10 transition-all font-medium"
            />
            <p className="text-right text-xs text-muted-foreground">{name.length}/30</p>
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
              onChange={e => setDescription(e.target.value)}
              placeholder={t('organization.permissions.config.roleDescriptionPlaceholder')}
              rows={4}
              maxLength={200}
              disabled={isLoading || saving}
              className="rounded-xl border focus:border-brand-main focus:ring-brand-main/10 transition-all resize-none font-medium p-4"
            />
            <p className="text-right text-xs text-muted-foreground">
              {description.length}/200
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
