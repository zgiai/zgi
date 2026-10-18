'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { WORKFLOW_AUTOSAVE_INTERVAL_MS } from '@/lib/config';

export type DigitalHumanDraftSaveState = 'saved' | 'dirty' | 'saving' | 'error';

export interface DigitalHumanDraftSaveOptions {
  silent?: boolean;
  force?: boolean;
}

interface DigitalHumanDraftSaveResult<TPayload> {
  savedPayload: TPayload;
  updatedAt: number;
}

interface UseDigitalHumanDraftPersistenceOptions<TPayload> {
  currentPayload: TPayload;
  initialPayload: TPayload;
  enabled: boolean;
  intervalMs?: number;
  canSave?: () => boolean;
  savePayload: (
    payload: TPayload,
    options: DigitalHumanDraftSaveOptions
  ) => Promise<DigitalHumanDraftSaveResult<TPayload>>;
}

export function useDigitalHumanDraftPersistence<TPayload>({
  currentPayload,
  initialPayload,
  enabled,
  intervalMs = WORKFLOW_AUTOSAVE_INTERVAL_MS,
  canSave,
  savePayload,
}: UseDigitalHumanDraftPersistenceOptions<TPayload>) {
  const [saveState, setSaveState] = useState<DigitalHumanDraftSaveState>('saved');
  const [lastSavedAt, setLastSavedAt] = useState<number | null>(null);
  const [isSaving, setIsSaving] = useState(false);
  const lastSavedSignatureRef = useRef(serialize(initialPayload));
  const currentPayloadRef = useRef(currentPayload);
  const currentSignatureRef = useRef(serialize(currentPayload));
  const savingRef = useRef(false);
  const canSaveRef = useRef(canSave);
  const savePayloadRef = useRef(savePayload);
  const currentSignature = useMemo(() => serialize(currentPayload), [currentPayload]);

  currentPayloadRef.current = currentPayload;
  currentSignatureRef.current = currentSignature;

  useEffect(() => {
    canSaveRef.current = canSave;
  }, [canSave]);

  useEffect(() => {
    savePayloadRef.current = savePayload;
  }, [savePayload]);

  const isDirty = Boolean(enabled && currentSignature !== lastSavedSignatureRef.current);

  useEffect(() => {
    if (!enabled || savingRef.current) return;
    setSaveState(isDirty ? 'dirty' : 'saved');
  }, [enabled, isDirty]);

  const saveNow = useCallback(
    async (options: DigitalHumanDraftSaveOptions = {}) => {
      if ((!enabled && !options.force) || savingRef.current) return false;
      if (canSaveRef.current && !canSaveRef.current()) {
        setSaveState('dirty');
        return false;
      }

      const submittedPayload = currentPayloadRef.current;
      const submittedSignature = currentSignatureRef.current;
      if (!options.force && submittedSignature === lastSavedSignatureRef.current) {
        setSaveState('saved');
        return true;
      }

      savingRef.current = true;
      setIsSaving(true);
      setSaveState('saving');
      try {
        const result = await savePayloadRef.current(submittedPayload, options);
        lastSavedSignatureRef.current = serialize(result.savedPayload);
        setLastSavedAt(result.updatedAt);
        if (currentSignatureRef.current === submittedSignature) {
          setSaveState('saved');
          return true;
        }
        setSaveState('dirty');
        return false;
      } catch {
        setSaveState('error');
        return false;
      } finally {
        savingRef.current = false;
        setIsSaving(false);
      }
    },
    [enabled]
  );

  useEffect(() => {
    if (!enabled) return;
    const intervalID = window.setInterval(() => {
      if (savingRef.current) return;
      if (currentSignatureRef.current === lastSavedSignatureRef.current) return;
      if (canSaveRef.current && !canSaveRef.current()) return;
      void saveNow({ silent: true });
    }, intervalMs);
    return () => window.clearInterval(intervalID);
  }, [enabled, intervalMs, saveNow]);

  return { saveState, lastSavedAt, isDirty, isSaving, saveNow };
}

function serialize(value: unknown) {
  return JSON.stringify(value);
}
