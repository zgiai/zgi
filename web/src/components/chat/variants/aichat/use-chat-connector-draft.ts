'use client';

import { useCallback, useMemo, useRef, useState } from 'react';
import type { AIChatInitialConnectorBinding } from '@/services/types/aichat';
import {
  chatConnectorDraftAttention,
  newChatConnectorDraft,
  reconcileChatConnectorDraft,
  sealChatConnectorDraft,
  synchronizeChatConnectorDefaults,
  updateChatConnectorDraftBindings,
  type ChatConnectorDefaultsQuery,
} from './connector-draft-model';

export function useChatConnectorDraft(
  scope: string,
  conversationId: string | null,
  defaults?: ChatConnectorDefaultsQuery
) {
  const [stored, setStored] = useState(() => newChatConnectorDraft(scope, conversationId));
  const draft = synchronizeChatConnectorDefaults(
    reconcileChatConnectorDraft(stored, scope, conversationId),
    defaults
  );
  // Reconcile during render so a changed account/workspace/target cannot expose
  // old choices for one frame or accept a late setup completion before an effect.
  if (draft !== stored) setStored(draft);
  const latest = useRef(draft);
  latest.current = draft;

  const update = useCallback(
    (bindings: AIChatInitialConnectorBinding[]) => {
      if (latest.current.id !== draft.id) return;
      const next = updateChatConnectorDraftBindings(latest.current, bindings);
      latest.current = next;
      setStored(next);
    },
    [draft.id]
  );
  const reset = useCallback(() => {
    const next = newChatConnectorDraft(scope, conversationId);
    latest.current = next;
    setStored(next);
  }, [scope, conversationId]);
  const capture = useCallback(() => {
    if (latest.current.id !== draft.id) return;
    const next = sealChatConnectorDraft(latest.current);
    latest.current = next;
    setStored(next);
    return {
      bindings: next.bindings.map(binding => ({ ...binding })),
      isCurrent: () => latest.current === next,
    };
  }, [draft.id]);
  const isCurrent = useCallback(() => latest.current === draft, [draft]);
  return useMemo(
    () => ({
      draft,
      update,
      reset,
      isCurrent,
      capture,
      attention: chatConnectorDraftAttention(draft),
    }),
    [draft, update, reset, isCurrent, capture]
  );
}
