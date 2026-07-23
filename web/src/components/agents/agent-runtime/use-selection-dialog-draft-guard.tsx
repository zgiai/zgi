'use client';

import { useCallback, useEffect, useState } from 'react';
import { UnsavedChangesConfirmDialog } from '@/components/ui/unsaved-changes-confirm-dialog';
import { useT } from '@/i18n';

interface SelectionDialogDraftGuardOptions {
  open: boolean;
  isDirty: boolean;
  disabled?: boolean;
  onOpenChange: (open: boolean) => void;
  onSave: () => void;
}

export function useSelectionDialogDraftGuard({
  open,
  isDirty,
  disabled = false,
  onOpenChange,
  onSave,
}: SelectionDialogDraftGuardOptions) {
  const t = useT('agents.agentRuntime.selectionDialog.closeGuard');
  const [confirmOpen, setConfirmOpen] = useState(false);

  useEffect(() => {
    if (!open) setConfirmOpen(false);
  }, [open]);

  const discardAndClose = useCallback(() => {
    setConfirmOpen(false);
    onOpenChange(false);
  }, [onOpenChange]);

  const saveAndClose = useCallback(() => {
    if (disabled) return;
    onSave();
    setConfirmOpen(false);
    onOpenChange(false);
  }, [disabled, onOpenChange, onSave]);

  const requestOpenChange = useCallback(
    (nextOpen: boolean) => {
      if (nextOpen) {
        onOpenChange(true);
        return;
      }
      if (isDirty) {
        setConfirmOpen(true);
        return;
      }
      onOpenChange(false);
    },
    [isDirty, onOpenChange]
  );

  const closeGuard = (
    <UnsavedChangesConfirmDialog
      open={confirmOpen}
      onOpenChange={setConfirmOpen}
      title={t('title')}
      description={t('description')}
      discardText={t('discard')}
      cancelText={t('continueEditing')}
      confirmText={t('save')}
      confirmDisabled={disabled}
      onDiscard={discardAndClose}
      onConfirm={saveAndClose}
    />
  );

  return {
    requestOpenChange,
    requestClose: () => requestOpenChange(false),
    saveAndClose,
    closeGuard,
  };
}
