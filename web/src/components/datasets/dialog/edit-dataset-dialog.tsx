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
import { useUpdateDataset } from '@/hooks/dataset/use-datasets';
import { useT } from '@/i18n';
import type { Dataset } from '@/services/types/dataset';
import { buildIconValueFromDataset, iconValueToDatasetPayload } from '@/utils/icon-helpers';
import { getNameValidationErrors, isValidNameInput } from '@/utils/validation';
import { cn } from '@/lib/utils';
import { ICON_BG, ICON_TEXT } from '@/lib/config';

interface EditDatasetDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  dataset?: Dataset;
}

/**
 * @component EditDatasetDialog
 * @category Feature
 * @status Stable
 * @description Edits dataset basic information only: name, description, and icon.
 * @usage Use from dataset card actions for lightweight metadata editing.
 * @example
 * <EditDatasetDialog open={open} onOpenChange={setOpen} dataset={dataset} />
 */
export function EditDatasetDialog({ open, onOpenChange, dataset }: EditDatasetDialogProps) {
  const t = useT();
  const updateDataset = useUpdateDataset(dataset?.id || '');
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [iconValue, setIconValue] = useState<IconValue>(createTextIconValue('', ICON_BG));
  const [hasSubmitted, setHasSubmitted] = useState(false);

  useEffect(() => {
    if (!open || !dataset) return;
    setName(dataset.name || '');
    setDescription(dataset.description || '');
    setIconValue(buildIconValueFromDataset(dataset));
    setHasSubmitted(false);
  }, [dataset, open]);

  const nameErrors = useMemo(() => getNameValidationErrors(name, { allowSpace: true }), [name]);
  const isNameValid = useMemo(() => isValidNameInput(name, { allowSpace: true }), [name]);

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    setHasSubmitted(true);
    if (!dataset || !isNameValid) return;

    await updateDataset.mutateAsync({
      name,
      description,
      ...iconValueToDatasetPayload(iconValue, {
        existing: dataset,
        defaultTextFromName: name,
      }),
    });
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg" aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-xl font-semibold">
            <Pencil className="h-5 w-5" />
            {t('datasets.editDataset')}
          </DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit}>
          <DialogBody className="space-y-5 pb-4">
            <div className="space-y-2.5">
              <Label className="flex items-center gap-1 text-sm font-semibold">
                {t('datasets.createModal.nameLabel')} <span className="text-destructive">*</span>
              </Label>
              <Input
                value={name}
                onChange={event => setName(event.target.value)}
                placeholder={t('datasets.createModal.namePlaceholder')}
                className={cn(
                  hasSubmitted &&
                    !isNameValid &&
                    'border-destructive focus-visible:ring-destructive'
                )}
                errorText={
                  hasSubmitted && !isNameValid
                    ? t(`datasets.validation.name.${nameErrors[0] || 'required'}`)
                    : undefined
                }
              />
            </div>

            <div className="space-y-2.5">
              <Label className="text-sm font-semibold">
                {t('datasets.createModal.descriptionLabel')}
              </Label>
              <Textarea
                value={description}
                onChange={event => setDescription(event.target.value)}
                placeholder={t('datasets.createModal.descriptionPlaceholder')}
                rows={3}
                className="resize-none"
              />
            </div>

            <div className="space-y-2.5">
              <Label className="text-sm font-semibold">{t('datasets.createModal.iconLabel')}</Label>
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
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={updateDataset.isPending}>
              {updateDataset.isPending ? t('common.loading') : t('common.confirm')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export default EditDatasetDialog;
