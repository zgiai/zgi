'use client';

import React, { useEffect, useMemo, useState } from 'react';
import { Pencil } from 'lucide-react';

import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { IconInput } from '@/components/common/icon-input';
import { createTextIconValue, type IconValue } from '@/components/common/icon-input/types';
import { useUpdateDb } from '@/hooks/db/use-dbs';
import { useT } from '@/i18n';
import type { Db, UpdateDbRequest } from '@/services/types/db';
import { buildIconValueFromDataset, iconValueToDatasetPayload } from '@/utils/icon-helpers';
import { getNameValidationErrors, isValidNameInput } from '@/utils/validation';
import { cn } from '@/lib/utils';
import { ICON_BG, ICON_TEXT } from '@/lib/config';

interface EditDbDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  db?: Db;
}

/**
 * @component EditDbDialog
 * @category Feature
 * @status Stable
 * @description Edits database basic information only: name, description, and icon.
 * @usage Use from database card actions for lightweight metadata editing.
 * @example
 * <EditDbDialog open={open} onOpenChange={setOpen} db={db} />
 */
export function EditDbDialog({ open, onOpenChange, db }: EditDbDialogProps) {
  const { dbs: t, common } = useT();
  const updateDb = useUpdateDb(db?.id);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [iconValue, setIconValue] = useState<IconValue>(createTextIconValue('', ICON_BG));
  const [hasSubmitted, setHasSubmitted] = useState(false);

  useEffect(() => {
    if (!open || !db) return;
    setName(db.name || '');
    setDescription(db.description || '');
    setIconValue(
      buildIconValueFromDataset({
        name: db.name,
        icon_type: db.icon_type || undefined,
        icon: db.icon || undefined,
        icon_url: db.icon_url || undefined,
        icon_background: db.icon_background || undefined,
      })
    );
    setHasSubmitted(false);
  }, [db, open]);

  const nameErrors = useMemo(() => getNameValidationErrors(name, { allowSpace: true }), [name]);
  const isNameValid = useMemo(() => isValidNameInput(name, { allowSpace: true }), [name]);

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    setHasSubmitted(true);
    if (!db || !isNameValid) return;

    const iconPayload = iconValueToDatasetPayload(iconValue, {
      existing: {
        icon: db.icon || null,
        icon_background: db.icon_background || null,
      },
      defaultTextFromName: name,
    });

    await updateDb.mutateAsync({
      name,
      description,
      icon_type: iconPayload.icon_type,
      icon: iconPayload.icon,
      icon_background: iconPayload.icon_background,
    } as UpdateDbRequest);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg" aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-xl font-semibold">
            <Pencil className="h-5 w-5" />
            {t('edit')}
          </DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit}>
          <DialogBody className="space-y-5 pb-4">
            <div className="space-y-2.5">
              <Label className="flex items-center gap-1 text-sm font-semibold">
                {t('createModal.nameLabel')} <span className="text-destructive">*</span>
              </Label>
              <Input
                value={name}
                onChange={event => setName(event.target.value)}
                placeholder={t('createModal.namePlaceholder')}
                className={cn(
                  hasSubmitted &&
                    !isNameValid &&
                    'border-destructive focus-visible:ring-destructive'
                )}
                errorText={
                  hasSubmitted && !isNameValid
                    ? t(`validation.name.${nameErrors[0] || 'required'}`)
                    : undefined
                }
              />
            </div>

            <div className="space-y-2">
              <Label className="flex items-center gap-1">{t('createModal.descriptionLabel')}</Label>
              <Textarea
                value={description}
                onChange={event => setDescription(event.target.value)}
                placeholder={t('createModal.descriptionPlaceholder')}
                rows={3}
                className="resize-none"
              />
            </div>

            <div className="space-y-2">
              <Label>{t('createModal.iconLabel')}</Label>
              <IconInput
                value={iconValue}
                defaultValue={createTextIconValue(
                  name.slice(0, 2).toUpperCase() || ICON_TEXT,
                  ICON_BG
                )}
                onChange={setIconValue}
              />
            </div>
          </DialogBody>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {common('cancel')}
            </Button>
            <Button type="submit" disabled={updateDb.isPending}>
              {updateDb.isPending ? common('loading') : common('confirm')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export default EditDbDialog;
