'use client';

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { cn } from '@/lib/utils';
import { IconInput } from '@/components/common/icon-input';
import { createTextIconValue, type IconValue } from '@/components/common/icon-input/types';
import { buildIconValueFromDataset, iconValueToDatasetPayload } from '@/utils/icon-helpers';
import { isValidNameInput, getNameValidationErrors } from '@/utils/validation';
import type { Db, CreateDbRequest, UpdateDbRequest } from '@/services/types/db';
import { useCreateDb, useUpdateDb } from '@/hooks/db/use-dbs';
import { useT } from '@/i18n';
import { ChevronLeft, Database as DatabaseIcon, Pencil } from 'lucide-react';
import { ICON_BG, ICON_TEXT } from '@/lib/config';

import { useCurrentWorkspace, type Workspace } from '@/store/workspace-store';

interface CreateDbDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export default function CreateDbDialog({ open, onOpenChange }: CreateDbDialogProps) {
  const { dbs: t, common } = useT();
  const isEdit = false;
  const db = undefined as Db | undefined;
  const currentWorkspace = useCurrentWorkspace();

  const createDb = useCreateDb();
  const updateDb = useUpdateDb(db?.id);

  const getDbIconValue = useCallback((dbData: typeof db): IconValue => {
    if (dbData) {
      return buildIconValueFromDataset({
        name: dbData.name,
        icon_type: dbData.icon_type || undefined,
        icon: dbData.icon || undefined,
        icon_url: dbData.icon_url || undefined,
        icon_background: dbData.icon_background || undefined,
      });
    }
    return createTextIconValue('', ICON_BG);
  }, []);

  const initialIconValue: IconValue = useMemo(() => {
    if (isEdit && db) {
      return getDbIconValue(db);
    }
    return createTextIconValue('', ICON_BG);
  }, [isEdit, db, getDbIconValue]);

  const buildFormDataFromDb = useCallback(
    (dbData?: Db) => {
      if (isEdit && dbData) {
        return {
          name: dbData.name || '',
          description: dbData.description || '',
          workspace: dbData.workspace_id ? { id: dbData.workspace_id, name: '' } : undefined,
        };
      }

      return {
        name: '',
        description: '',
        workspace: undefined,
      };
    },
    [isEdit]
  );

  const [iconValue, setIconValue] = useState<IconValue>(initialIconValue);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [formData, setFormData] = useState<{
    name: string;
    description: string;
    workspace?: Workspace;
  }>(() => buildFormDataFromDb(db));

  useEffect(() => {
    if (!open) return;
    if (currentWorkspace) {
      if (formData.workspace?.id !== currentWorkspace.id) {
        setFormData(prev => ({
          ...prev,
          workspace: { id: currentWorkspace.id, name: currentWorkspace.name },
        }));
      }
      return;
    }
    // Guard: only default to first workspace in create mode.
    // In edit mode, respect db.workspace_id initialization to avoid overriding.
    if (isEdit) return;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, isEdit, formData.workspace, currentWorkspace]);

  function handleInputChange<K extends keyof typeof formData>(
    field: K,
    value: (typeof formData)[K]
  ) {
    setFormData(prev => ({ ...prev, [field]: value }));
  }

  // Validate name: 2-32 Unicode chars; letters, numbers, underscore, hyphen, optional spaces
  const isNameValid = useMemo(
    () => isValidNameInput(formData.name, { allowSpace: true }),
    [formData.name]
  );
  const nameErrors = useMemo(
    () => getNameValidationErrors(formData.name, { allowSpace: true }),
    [formData.name]
  );

  // Track if form has been submitted to show validation errors only after submit
  const [hasSubmitted, setHasSubmitted] = useState(false);

  // Reset hasSubmitted when dialog opens
  useEffect(() => {
    if (open) {
      setHasSubmitted(false);
    }
  }, [open]);

  // Sync form and icon state when opening in edit mode or switching to another db.
  useEffect(() => {
    if (!open) return;

    if (isEdit && db) {
      setFormData(buildFormDataFromDb(db));
      setIconValue(getDbIconValue(db));
    } else if (!isEdit) {
      setFormData(buildFormDataFromDb());
      setIconValue(createTextIconValue('', ICON_BG));
    }
  }, [open, isEdit, db, buildFormDataFromDb, getDbIconValue]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setHasSubmitted(true);
    const workspaceId = formData.workspace?.id || db?.workspace_id || currentWorkspace?.id || '';
    if (!isNameValid || !workspaceId) return;

    const iconPayload = iconValueToDatasetPayload(iconValue, {
      existing: isEdit
        ? { icon: db?.icon || null, icon_background: db?.icon_background || null }
        : undefined,
      defaultTextFromName: formData.name,
    });

    try {
      if (isEdit && db) {
        await updateDb.mutateAsync({
          name: formData.name,
          description: formData.description,
          workspace_id: workspaceId,
          icon_type: iconPayload.icon_type,
          icon: iconPayload.icon,
          icon_background: iconPayload.icon_background,
        } as UpdateDbRequest);
        onOpenChange(false);
      } else {
        await createDb.mutateAsync({
          name: formData.name,
          description: formData.description,
          workspace_id: workspaceId,
          icon_type: iconPayload.icon_type,
          icon: iconPayload.icon,
          icon_background: iconPayload.icon_background,
        } as CreateDbRequest);
        onOpenChange(false);
        setFormData({ name: '', description: '', workspace: undefined });
        setIconValue(createTextIconValue('', ICON_BG));
      }
    } catch (error) {
      console.error('CreateDbDialog submit error:', error);
    }
  };

  const resetCreateState = () => {
    setFormData({ name: '', description: '', workspace: undefined });
    setIconValue(createTextIconValue('', ICON_BG));
    setHasSubmitted(false);
  };

  const handleDialogOpenChange = (nextOpen: boolean) => {
    if (!nextOpen && !isEdit) {
      resetCreateState();
    }
    onOpenChange(nextOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleDialogOpenChange}>
      <DialogContent className="max-w-lg" aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-xl font-semibold">
            {isEdit ? <Pencil className="h-5 w-5" /> : <DatabaseIcon className="h-5 w-5" />}
            {isEdit ? t('edit') : t('create')}
          </DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit}>
          <DialogBody className="space-y-6">
            <div className="space-y-6">
              {/* Name */}
              <div className="space-y-2.5">
                <Label className="flex items-center gap-1 text-sm font-semibold">
                  {t('createModal.nameLabel')} <span className="text-destructive">*</span>
                </Label>
                <Input
                  placeholder={t('createModal.namePlaceholder')}
                  value={formData.name}
                  onChange={e => handleInputChange('name', e.target.value)}
                  required
                  className={cn(
                    'h-11',
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

              {/* Description */}
              <div className="space-y-2">
                <Label className="flex items-center gap-1">
                  {t('createModal.descriptionLabel')}
                </Label>
                <Textarea
                  placeholder={t('createModal.descriptionPlaceholder')}
                  value={formData.description}
                  onChange={e => handleInputChange('description', e.target.value)}
                  rows={4}
                  className="resize-none"
                />
              </div>

              {/* Advanced Settings */}
              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <button
                    type="button"
                    onClick={() => setShowAdvanced(prev => !prev)}
                    className="flex items-center gap-2 text-sm font-semibold hover:text-primary transition-colors focus:outline-none"
                  >
                    <ChevronLeft
                      className={cn(
                        'size-4 transition-transform duration-300',
                        showAdvanced ? '-rotate-90' : ''
                      )}
                    />
                    {t('createModal.advancedSettingsLabel')}
                  </button>
                </div>

                <div
                  className={cn(
                    'space-y-4 w-full transition-all duration-300 overflow-hidden',
                    showAdvanced ? 'h-auto opacity-100 py-4' : 'h-0 opacity-0'
                  )}
                >
                  {/* Icon */}
                  <div className="space-y-2">
                    <Label>{t('createModal.iconLabel')}</Label>
                    <IconInput
                      value={iconValue}
                      defaultValue={createTextIconValue(
                        (formData.name || '').slice(0, 2).toUpperCase() || ICON_TEXT,
                        ICON_BG
                      )}
                      onChange={setIconValue}
                    />
                  </div>
                </div>
              </div>
            </div>

            {/* Footer */}
            <div className="flex justify-center pb-2">
              <Button
                type="submit"
                className="px-8"
                disabled={isEdit ? updateDb.isPending : createDb.isPending}
              >
                {isEdit
                  ? common('confirm')
                  : createDb.isPending
                    ? t('createModal.creatingButton')
                    : t('createModal.createButton')}
              </Button>
            </div>
          </DialogBody>
        </form>
      </DialogContent>
    </Dialog>
  );
}
