'use client';

import { useCallback, useMemo, useReducer } from 'react';

import type { ApprovalRuntimeForm as ApprovalRuntimeFormData } from '@/services/approval.service';
import {
  getApprovalEventSequence,
  parseApprovalExpiredEvent,
  parseApprovalPausedEvent,
  parseApprovalRequestedEvent,
  parseApprovalResultFilledEvent,
  type ParsedApprovalRuntimeEntry,
} from './runtime-events';

export type ApprovalStatus = 'waiting' | 'submitting' | 'submitted' | 'expired' | 'completed';

export interface ApprovalEntry {
  key: string;
  formId: string;
  nodeId: string;
  nodeTitle: string;
  token: string | null;
  form: ApprovalRuntimeFormData | null;
  status: ApprovalStatus;
  submittedAction: string | null;
  lastSequence: number;
}

export interface ApprovalRuntimeState {
  byKey: Record<string, ApprovalEntry>;
  activeKey: string | null;
  cursor: number;
}

type ApprovalRuntimeAction =
  | {
      type: 'approval_requested';
      entry: ParsedApprovalRuntimeEntry;
      sequence: number | null;
    }
  | {
      type: 'workflow_paused';
      entries: ParsedApprovalRuntimeEntry[];
      sequence: number | null;
    }
  | {
      type: 'approval_result_filled';
      entry: ParsedApprovalRuntimeEntry;
      sequence: number | null;
    }
  | {
      type: 'approval_expired';
      entry: ParsedApprovalRuntimeEntry;
      sequence: number | null;
    }
  | {
      type: 'form_loaded';
      form: ApprovalRuntimeFormData;
    }
  | {
      type: 'submitting';
      key: string;
      action: string;
    }
  | {
      type: 'submitted';
      key: string;
      action: string;
    }
  | {
      type: 'waiting';
      key: string;
    }
  | { type: 'reset' };

export const APPROVAL_RUNTIME_INITIAL_STATE: ApprovalRuntimeState = {
  byKey: {},
  activeKey: null,
  cursor: 0,
};

export function hasUnresolvedApprovalEntries(state: ApprovalRuntimeState) {
  return Object.values(state.byKey).some(entry => entry.status === 'waiting');
}

function getApprovalKey(entry: Pick<ParsedApprovalRuntimeEntry, 'formId' | 'form' | 'nodeId'>) {
  return entry.formId || entry.form?.id || entry.nodeId;
}

function getFormKey(form: ApprovalRuntimeFormData) {
  return form.id || form.node_id;
}

function getNextWaitingKey(byKey: Record<string, ApprovalEntry>, excludeKey?: string) {
  return (
    Object.values(byKey).find(entry => entry.key !== excludeKey && entry.status === 'waiting')
      ?.key ?? null
  );
}

function shouldReplaceActive(state: ApprovalRuntimeState, nextByKey: Record<string, ApprovalEntry>) {
  if (!state.activeKey) return true;
  const active = nextByKey[state.activeKey];
  return (
    !active ||
    active.status === 'submitted' ||
    active.status === 'expired' ||
    active.status === 'completed'
  );
}

function advanceCursor(state: ApprovalRuntimeState, sequence: number | null) {
  return sequence === null ? state.cursor : Math.max(state.cursor, sequence);
}

function upsertEntry(
  state: ApprovalRuntimeState,
  entry: ParsedApprovalRuntimeEntry,
  sequence: number | null,
  status: ApprovalStatus
): ApprovalRuntimeState {
  const key = getApprovalKey(entry);
  if (!key) return { ...state, cursor: advanceCursor(state, sequence) };

  const previous = state.byKey[key];
  const nextEntry: ApprovalEntry = {
    key,
    formId: entry.formId || previous?.formId || entry.form?.id || '',
    nodeId: entry.nodeId || previous?.nodeId || entry.form?.node_id || '',
    nodeTitle: entry.nodeTitle || previous?.nodeTitle || entry.form?.node_title || '',
    token: entry.token || previous?.token || entry.form?.token || null,
    form: entry.form || previous?.form || null,
    status,
    submittedAction:
      status === 'waiting'
        ? null
        : entry.actionId || previous?.submittedAction || null,
    lastSequence:
      sequence === null ? previous?.lastSequence ?? 0 : Math.max(previous?.lastSequence ?? 0, sequence),
  };
  const byKey = { ...state.byKey, [key]: nextEntry };
  const activeKey = shouldReplaceActive(state, byKey) ? key : state.activeKey;
  return {
    byKey,
    activeKey,
    cursor: advanceCursor(state, sequence),
  };
}

function updateResolvedEntry(
  state: ApprovalRuntimeState,
  entry: ParsedApprovalRuntimeEntry,
  sequence: number | null,
  status: ApprovalStatus
): ApprovalRuntimeState {
  const key = getApprovalKey(entry);
  if (!key) return { ...state, cursor: advanceCursor(state, sequence) };

  const previous = state.byKey[key];
  const nextEntry: ApprovalEntry = {
    key,
    formId: entry.formId || previous?.formId || '',
    nodeId: entry.nodeId || previous?.nodeId || '',
    nodeTitle: entry.nodeTitle || previous?.nodeTitle || '',
    token: entry.token || previous?.token || null,
    form: previous?.form || entry.form || null,
    status,
    submittedAction: entry.actionId || previous?.submittedAction || null,
    lastSequence:
      sequence === null ? previous?.lastSequence ?? 0 : Math.max(previous?.lastSequence ?? 0, sequence),
  };
  const byKey = { ...state.byKey, [key]: nextEntry };
  const activeKey =
    state.activeKey === key ? getNextWaitingKey(byKey, key) ?? key : state.activeKey;
  return {
    byKey,
    activeKey,
    cursor: advanceCursor(state, sequence),
  };
}

export function approvalRuntimeReducer(
  state: ApprovalRuntimeState,
  action: ApprovalRuntimeAction
): ApprovalRuntimeState {
  switch (action.type) {
    case 'approval_requested':
      return upsertEntry(state, action.entry, action.sequence, 'waiting');
    case 'workflow_paused':
      return action.entries.reduce(
        (next, entry) => upsertEntry(next, entry, action.sequence, 'waiting'),
        { ...state, cursor: advanceCursor(state, action.sequence) }
      );
    case 'approval_result_filled':
      return updateResolvedEntry(state, action.entry, action.sequence, 'submitted');
    case 'approval_expired':
      return updateResolvedEntry(state, action.entry, action.sequence, 'expired');
    case 'form_loaded': {
      const key = getFormKey(action.form);
      if (!key) return state;
      const previous = state.byKey[key];
      const byKey = {
        ...state.byKey,
        [key]: {
          key,
          formId: action.form.id || previous?.formId || '',
          nodeId: action.form.node_id || previous?.nodeId || '',
          nodeTitle: action.form.node_title || previous?.nodeTitle || '',
          token: action.form.token || previous?.token || null,
          form: action.form,
          status: previous?.status ?? 'waiting',
          submittedAction: previous?.submittedAction ?? null,
          lastSequence: previous?.lastSequence ?? state.cursor,
        },
      };
      return {
        ...state,
        byKey,
        activeKey: shouldReplaceActive(state, byKey) ? key : state.activeKey,
      };
    }
    case 'submitting': {
      const previous = state.byKey[action.key];
      if (!previous) return state;
      return {
        ...state,
        byKey: {
          ...state.byKey,
          [action.key]: {
            ...previous,
            status: 'submitting',
            submittedAction: action.action,
          },
        },
        activeKey: action.key,
      };
    }
    case 'submitted': {
      const previous = state.byKey[action.key];
      if (!previous) return state;
      const byKey = {
        ...state.byKey,
        [action.key]: {
          ...previous,
          status: 'submitted' as const,
          submittedAction: action.action,
        },
      };
      return {
        ...state,
        byKey,
        activeKey:
          state.activeKey === action.key
            ? getNextWaitingKey(byKey, action.key) ?? action.key
            : state.activeKey,
      };
    }
    case 'waiting': {
      const previous = state.byKey[action.key];
      if (!previous) return state;
      return {
        ...state,
        byKey: {
          ...state.byKey,
          [action.key]: {
            ...previous,
            status: 'waiting',
            submittedAction: null,
          },
        },
        activeKey: action.key,
      };
    }
    case 'reset':
      return APPROVAL_RUNTIME_INITIAL_STATE;
    default:
      return state;
  }
}

function getEnvelopeEvent(payload: unknown, fallbackEvent?: string) {
  const record =
    payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : {};
  return typeof record.event === 'string' ? record.event : fallbackEvent || '';
}

function getEnvelopeSequence(payload: unknown) {
  const record =
    payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : {};
  return getApprovalEventSequence(record);
}

/**
 * @hook useApprovalRuntimeEvents
 * @category Workflow
 * @status Beta
 * @description Form-scoped runtime state for human approval workflow events.
 */
export function useApprovalRuntimeEvents() {
  const [state, dispatch] = useReducer(approvalRuntimeReducer, APPROVAL_RUNTIME_INITIAL_STATE);

  const activeEntry = state.activeKey ? state.byKey[state.activeKey] || null : null;
  const activeForm = activeEntry?.form ?? null;
  const activeToken = activeEntry?.token ?? null;
  const submittedAction = activeEntry?.submittedAction ?? null;
  const isSubmitting =
    activeEntry?.status === 'submitting' || Boolean(activeEntry?.submittedAction);
  const isExpired = activeEntry?.status === 'expired';
  const isPending = useMemo(
    () =>
      Boolean(
        activeEntry ||
          Object.values(state.byKey).some(entry =>
            ['waiting', 'submitting', 'submitted'].includes(entry.status)
          )
      ),
    [activeEntry, state.byKey]
  );

  const dispatchApprovalEvent = useCallback((eventName: string, payload: unknown) => {
    const event = getEnvelopeEvent(payload, eventName);
    const sequence = getEnvelopeSequence(payload);
    switch (event) {
      case 'approval_requested': {
        const parsed = parseApprovalRequestedEvent(payload);
        dispatch({
          type: 'approval_requested',
          entry: {
            formId: parsed.formId,
            nodeId: parsed.nodeId,
            nodeTitle: parsed.nodeTitle,
            token: parsed.token,
            form: parsed.form,
            actionId: parsed.actionId,
          },
          sequence,
        });
        break;
      }
      case 'workflow_paused': {
        const parsed = parseApprovalPausedEvent(payload);
        if (!parsed.isApproval) break;
        dispatch({ type: 'workflow_paused', entries: parsed.entries, sequence });
        break;
      }
      case 'approval_result_filled': {
        const parsed = parseApprovalResultFilledEvent(payload);
        dispatch({
          type: 'approval_result_filled',
          entry: {
            formId: parsed.formId,
            nodeId: parsed.nodeId,
            nodeTitle: parsed.nodeTitle,
            token: parsed.token,
            form: null,
            actionId: parsed.actionId,
          },
          sequence,
        });
        break;
      }
      case 'approval_expired': {
        const parsed = parseApprovalExpiredEvent(payload);
        dispatch({
          type: 'approval_expired',
          entry: {
            formId: parsed.formId,
            nodeId: parsed.nodeId,
            nodeTitle: parsed.nodeTitle,
            token: parsed.token,
            form: null,
            actionId: null,
          },
          sequence,
        });
        break;
      }
      case 'workflow_finished':
      case 'workflow_stopped':
      case 'workflow_failed':
      case 'workflow_succeeded':
      case 'workflow_completed':
      case 'error':
        dispatch({ type: 'reset' });
        break;
    }
  }, []);

  const setSubmitting = useCallback((key: string, action: string) => {
    dispatch({ type: 'submitting', key, action });
  }, []);

  const setSubmitted = useCallback((key: string, action: string) => {
    dispatch({ type: 'submitted', key, action });
  }, []);

  const setWaiting = useCallback((key: string) => {
    dispatch({ type: 'waiting', key });
  }, []);

  const setLoadedForm = useCallback((form: ApprovalRuntimeFormData) => {
    dispatch({ type: 'form_loaded', form });
  }, []);

  const resetApprovalRuntime = useCallback(() => {
    dispatch({ type: 'reset' });
  }, []);

  return {
    state,
    activeEntry,
    activeForm,
    activeToken,
    submittedAction,
    isSubmitting,
    isExpired,
    isPending,
    dispatchApprovalEvent,
    setSubmitting,
    setSubmitted,
    setWaiting,
    setLoadedForm,
    resetApprovalRuntime,
  };
}
