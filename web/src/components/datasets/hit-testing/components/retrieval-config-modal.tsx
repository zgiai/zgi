'use client';

import React, { useState, useEffect } from 'react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { RetrievalSettings } from '@/components/datasets/indexing-config/retrieval-settings';
import type { RetrievalConfigModalProps, RetrievalConfig } from '../types';

/**
 * RetrievalConfigModal Component
 * Modal for configuring retrieval parameters and search methods
 * Now uses the shared RetrievalSettings component for consistency.
 */
export function RetrievalConfigModal({
  open,
  onOpenChange,
  config,
  onConfigChange,
  onSave,
  onSaveAsTest,
  isGraphEnabled = false,
}: RetrievalConfigModalProps) {
  const t = useT('datasets');
  const [localConfig, setLocalConfig] = useState<RetrievalConfig>(config);

  // Sync local state when parent config changes or when modal opens
  useEffect(() => {
    if (open) {
      setLocalConfig(config);
    }
  }, [config, open]);

  // Handle local config changes
  const handleConfigChange = (updates: Partial<RetrievalConfig>) => {
    const newConfig = { ...localConfig, ...updates };
    setLocalConfig(newConfig);
    onConfigChange(newConfig);
  };

  // Handle save to dataset settings (persists to backend)
  const handleSaveToSettings = () => {
    onSave(localConfig);
    onOpenChange(false);
  };

  // Handle save as test config only (local state, not persisted)
  const handleSaveAsTest = () => {
    onSaveAsTest?.(localConfig);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl p-0 overflow-hidden flex flex-col max-h-[90vh]">
        <DialogHeader>
          <DialogTitle>{t('hitTesting.retrievalConfig')}</DialogTitle>
        </DialogHeader>

        <DialogBody>
          <RetrievalSettings
            retrieval={localConfig}
            onChange={updated => handleConfigChange(updated)}
            isGraphEnabled={isGraphEnabled}
          />
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t gap-3">
          <Button
            variant="outline"
            onClick={handleSaveToSettings}
            className="font-bold rounded-xl h-11 px-6 transition-all active:scale-95"
          >
            {t('hitTesting.saveToSettings')}
          </Button>
          <Button
            onClick={handleSaveAsTest}
            className="font-bold rounded-xl h-11 px-6 shadow-premium transition-all active:scale-95"
          >
            {t('hitTesting.saveAsTestOnly')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
