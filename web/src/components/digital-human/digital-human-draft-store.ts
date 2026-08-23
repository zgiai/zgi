'use client';

import { useEffect, useRef } from 'react';
import { create } from 'zustand';
import type { DigitalHumanDraftSaveState } from './use-digital-human-draft-persistence';

export interface DigitalHumanDraftEditorState {
  token: symbol;
  canUpdate: boolean;
  saveState: DigitalHumanDraftSaveState;
  isDirty: boolean;
  isSaving: boolean;
  lastSavedAt: number | null;
  saveNow: () => Promise<boolean>;
}

interface DigitalHumanDraftStoreState {
  editors: Record<string, DigitalHumanDraftEditorState | undefined>;
  resetVersions: Record<string, number | undefined>;
  register: (id: string, editor: DigitalHumanDraftEditorState) => void;
  unregister: (id: string, token: symbol) => void;
  resetFromServer: (id: string) => void;
}

export const useDigitalHumanDraftStore = create<DigitalHumanDraftStoreState>(set => ({
  editors: {},
  resetVersions: {},
  register: (id, editor) => set(state => ({ editors: { ...state.editors, [id]: editor } })),
  unregister: (id, token) =>
    set(state => {
      if (state.editors[id]?.token !== token) return state;
      const editors = { ...state.editors };
      delete editors[id];
      return { editors };
    }),
  resetFromServer: id =>
    set(state => ({
      resetVersions: {
        ...state.resetVersions,
        [id]: (state.resetVersions[id] ?? 0) + 1,
      },
    })),
}));

export function useRegisterDigitalHumanDraftEditor(
  id: string,
  editor: Omit<DigitalHumanDraftEditorState, 'token'>
) {
  const tokenRef = useRef(Symbol(`digital-human-draft:${id}`));
  const register = useDigitalHumanDraftStore(state => state.register);
  const unregister = useDigitalHumanDraftStore(state => state.unregister);

  useEffect(() => {
    const token = tokenRef.current;
    register(id, {
      token,
      canUpdate: editor.canUpdate,
      saveState: editor.saveState,
      isDirty: editor.isDirty,
      isSaving: editor.isSaving,
      lastSavedAt: editor.lastSavedAt,
      saveNow: editor.saveNow,
    });
  }, [
    editor.canUpdate,
    editor.isDirty,
    editor.isSaving,
    editor.lastSavedAt,
    editor.saveNow,
    editor.saveState,
    id,
    register,
  ]);

  useEffect(() => {
    const token = tokenRef.current;
    return () => unregister(id, token);
  }, [id, unregister]);
}
