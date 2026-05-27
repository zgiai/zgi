'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { WORKFLOW_AUTOSAVE_INTERVAL_MS } from '@/lib/config';
import type { UpdateAgentRuntimeConfigRequest } from '@/services/types/agent';
import type { AgentRuntimeSaveState } from './types';
import { buildAgentRuntimeSignature } from './utils';

interface AgentRuntimeSaveResult {
  savedPayload: UpdateAgentRuntimeConfigRequest;
  updatedAt: number;
}

export interface AgentRuntimeDraftPersistenceSnapshot {
  lastSavedSignature: string;
  lastSavedAt: number | null;
  saveState: AgentRuntimeSaveState;
}

interface SaveNowOptions {
  silent?: boolean;
  force?: boolean;
}

interface UseAgentRuntimeDraftPersistenceOptions {
  currentPayload: UpdateAgentRuntimeConfigRequest;
  enabled: boolean;
  intervalMs?: number;
  canSave?: () => boolean;
  savePayload: (
    payload: UpdateAgentRuntimeConfigRequest,
    options: SaveNowOptions
  ) => Promise<AgentRuntimeSaveResult>;
  onSaveFailed?: (error: unknown, options: SaveNowOptions) => void;
}

export function useAgentRuntimeDraftPersistence({
  currentPayload,
  enabled,
  intervalMs = WORKFLOW_AUTOSAVE_INTERVAL_MS,
  canSave,
  savePayload,
  onSaveFailed,
}: UseAgentRuntimeDraftPersistenceOptions) {
  const [saveState, setSaveState] = useState<AgentRuntimeSaveState>('idle');
  const [lastSavedAt, setLastSavedAt] = useState<number | null>(null);
  const [isSaving, setIsSaving] = useState(false);
  const lastSavedSignatureRef = useRef('');
  const currentSignatureRef = useRef('');
  const canSaveRef = useRef(canSave);
  const savePayloadRef = useRef(savePayload);
  const onSaveFailedRef = useRef(onSaveFailed);
  const currentSignature = useMemo(
    () => buildAgentRuntimeSignature(currentPayload),
    [currentPayload]
  );

  currentSignatureRef.current = currentSignature;

  useEffect(() => {
    canSaveRef.current = canSave;
  }, [canSave]);

  useEffect(() => {
    savePayloadRef.current = savePayload;
  }, [savePayload]);

  useEffect(() => {
    onSaveFailedRef.current = onSaveFailed;
  }, [onSaveFailed]);

  const isDirty = Boolean(
    enabled && lastSavedSignatureRef.current && currentSignature !== lastSavedSignatureRef.current
  );

  useEffect(() => {
    if (!enabled || isSaving) return;
    if (!lastSavedSignatureRef.current) return;
    setSaveState(isDirty ? 'dirty' : 'saved');
  }, [enabled, isDirty, isSaving]);

  const markHydrated = useCallback(
    (payload: UpdateAgentRuntimeConfigRequest, updatedAt: number | null) => {
      lastSavedSignatureRef.current = buildAgentRuntimeSignature(payload);
      setLastSavedAt(updatedAt);
      setSaveState('saved');
    },
    []
  );

  const markServerSaved = useCallback(
    (payload: UpdateAgentRuntimeConfigRequest, updatedAt: number | null) => {
      lastSavedSignatureRef.current = buildAgentRuntimeSignature(payload);
      setLastSavedAt(updatedAt);
      setSaveState('saved');
    },
    []
  );

  const setPreviewing = useCallback(() => {
    setSaveState('previewing');
  }, []);

  const saveNow = useCallback(
    async (options: SaveNowOptions = {}) => {
      if (!enabled && !options.force) return false;
      if (canSaveRef.current && !canSaveRef.current()) {
        setSaveState('dirty');
        return false;
      }
      const submittedPayload = currentPayload;
      const submittedSignature = buildAgentRuntimeSignature(submittedPayload);

      if (!options.force && submittedSignature === lastSavedSignatureRef.current) {
        setSaveState('saved');
        return true;
      }

      setIsSaving(true);
      setSaveState('saving');

      try {
        const result = await savePayloadRef.current(submittedPayload, options);

        if (currentSignatureRef.current !== submittedSignature) {
          setSaveState('dirty');
          return false;
        }

        lastSavedSignatureRef.current = submittedSignature;
        setLastSavedAt(result.updatedAt);
        setSaveState('saved');
        return true;
      } catch (error) {
        if (currentSignatureRef.current === submittedSignature) {
          setSaveState('error');
        }
        onSaveFailedRef.current?.(error, options);
        return false;
      } finally {
        setIsSaving(false);
      }
    },
    [currentPayload, enabled]
  );

  const getSnapshot = useCallback(
    (): AgentRuntimeDraftPersistenceSnapshot => ({
      lastSavedSignature: lastSavedSignatureRef.current,
      lastSavedAt,
      saveState,
    }),
    [lastSavedAt, saveState]
  );

  const restoreSnapshot = useCallback((snapshot: AgentRuntimeDraftPersistenceSnapshot) => {
    lastSavedSignatureRef.current = snapshot.lastSavedSignature;
    setLastSavedAt(snapshot.lastSavedAt);
    setSaveState(snapshot.saveState);
  }, []);

  useEffect(() => {
    if (!enabled) return;

    const intervalID = window.setInterval(() => {
      if (!lastSavedSignatureRef.current) return;
      if (isSaving) return;
      if (currentSignatureRef.current === lastSavedSignatureRef.current) return;
      if (canSaveRef.current && !canSaveRef.current()) return;
      void saveNow({ silent: true });
    }, intervalMs);

    return () => {
      window.clearInterval(intervalID);
    };
  }, [enabled, intervalMs, isSaving, saveNow]);

  return {
    saveState,
    lastSavedAt,
    isDirty,
    isSaving,
    saveNow,
    markHydrated,
    markServerSaved,
    setPreviewing,
    getSnapshot,
    restoreSnapshot,
  };
}
