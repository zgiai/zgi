'use client';

import React, { useEffect, useState } from 'react';
import { FolderPlus, Pencil } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogFooter,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import {
  useCreateDatasetFolder,
  useUpdateDatasetFolder,
} from '@/hooks/dataset/use-dataset-folders';
import { useT } from '@/i18n';
import type { DatasetFolder } from '@/services/types/dataset-folder';
import { getNameValidationErrors } from '@/utils/validation';
import { cn } from '@/lib/utils';
import { useCurrentWorkspace } from '@/store/workspace-store';

// Event payload for opening folder modal via event bus
export interface OpenFolderModalPayload {
  mode: 'create' | 'edit';
  folder?: DatasetFolder;
  parentFolderId?: string;
}

interface FolderModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: 'create' | 'edit';
  folder?: DatasetFolder;
  parentFolderId?: string; // used only in create mode
}

export default function FolderModal({
  open,
  onOpenChange,
  mode,
  folder,
  parentFolderId,
}: FolderModalProps) {
  const t = useT();
  const currentWorkspace = useCurrentWorkspace();

  const createFolderMutation = useCreateDatasetFolder();
  const updateFolderMutation = useUpdateDatasetFolder();

  const [formData, setFormData] = useState<{
    name: string;
    description: string;
  }>(() => {
    if (mode === 'edit' && folder) {
      return {
        name: folder.name || '',
        description: folder.description || '',
      };
    }
    return { name: '', description: '' };
  });

  // Reset base fields when modal opens.
  useEffect(() => {
    if (!open) return;
    if (mode === 'edit' && folder) {
      setFormData({
        name: folder.name || '',
        description: folder.description || '',
      });
    } else if (mode === 'create') {
      setFormData({
        name: '',
        description: '',
      });
    }
  }, [folder, open, mode]);

  // Strictly typed handler for input changes
  function handleInputChange<K extends 'name' | 'description'>(
    field: K,
    value: { name: string; description: string }[K]
  ): void {
    setFormData(prev => ({ ...prev, [field]: value }) as typeof prev);
  }

  // Inline name validation using shared utility
  const nameErrors = getNameValidationErrors(formData.name, { allowSpace: true });
  const isNameValid = nameErrors.length === 0;

  // Track if form has been submitted to show validation errors only after submit
  const [hasSubmitted, setHasSubmitted] = useState(false);

  // Reset hasSubmitted when modal opens
  useEffect(() => {
    if (open) {
      setHasSubmitted(false);
    }
  }, [open]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setHasSubmitted(true);
    if (!isNameValid) return;

    if (mode === 'create') {
      const workspaceId = currentWorkspace?.id || '';
      if (!workspaceId) return;

      try {
        await createFolderMutation.mutateAsync({
          name: formData.name,
          description: formData.description,
          workspace_id: workspaceId,
          parent_id: parentFolderId || null,
        });
        onOpenChange(false);
        // Reset form state
        setFormData({ name: '', description: '' });
      } catch (error) {
        console.error('Failed to create folder:', error);
      }
    } else if (mode === 'edit' && folder) {
      try {
        await updateFolderMutation.mutateAsync({
          folderId: folder.id,
          data: {
            name: formData.name,
            description: formData.description,
            workspace_id: currentWorkspace?.id || folder.workspace_id,
          },
        });
        onOpenChange(false);
      } catch (error) {
        console.error('Failed to update folder:', error);
      }
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg p-0 overflow-hidden">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-xl font-semibold">
            {mode === 'create' ? (
              <FolderPlus className="h-5 w-5" />
            ) : (
              <Pencil className="h-5 w-5" />
            )}
            {mode === 'create'
              ? t('datasets.createFolderModal.title')
              : t('datasets.editFolderModal.title')}
          </DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit}>
          <DialogBody className="space-y-6 py-6">
            <div className="grid grid-cols-1 gap-6">
              {/* Folder Name */}
              <div className="space-y-2.5">
                <Label className="flex items-center gap-1 text-sm font-semibold">
                  {t('datasets.createFolderModal.nameLabel')}{' '}
                  <span className="text-destructive">*</span>
                </Label>
                <Input
                  placeholder={t('datasets.createFolderModal.namePlaceholder')}
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
                      ? t(`datasets.validation.name.${nameErrors[0] || 'required'}`)
                      : undefined
                  }
                />
              </div>

              {/* Description */}
              <div className="space-y-2">
                <Label className="flex items-center gap-1">
                  {t('datasets.createFolderModal.descriptionLabel')}
                </Label>
                <Textarea
                  placeholder={t('datasets.createFolderModal.descriptionPlaceholder')}
                  value={formData.description}
                  onChange={e => handleInputChange('description', e.target.value)}
                  rows={4}
                  className="resize-none"
                />
              </div>

            </div>
          </DialogBody>

          <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t font-medium">
            <Button
              type="button"
              variant="ghost"
              onClick={() => onOpenChange(false)}
              className="font-semibold"
            >
              {t('common.cancel')}
            </Button>
            <Button type="submit" className="px-8 font-bold">
              {mode === 'create' ? t('datasets.createFolder') : t('datasets.save')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
