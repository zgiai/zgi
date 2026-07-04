import type { AIChatAgenticTimelineItem } from '@/components/chat/controllers/aichat/types';

export const AICHAT_TIMELINE_DEBUG_STORAGE_KEY = 'zgi:aichat:timeline-debug';
const AICHAT_TIMELINE_DEBUG_ELEMENT_ID = '__aichatTimelineDebugLogs';
const AICHAT_TIMELINE_DEBUG_MAX_RECORDS = 160;

export function isAIChatTimelineDebugEnabled(): boolean {
  if (typeof window === 'undefined') return false;
  try {
    if (new URLSearchParams(window.location.search).get('aichatTimelineDebug') === '1') {
      return true;
    }
  } catch {
    // Fall through to localStorage below.
  }
  try {
    const value = window.localStorage.getItem(AICHAT_TIMELINE_DEBUG_STORAGE_KEY);
    return value === '1' || value === 'true';
  } catch {
    return false;
  }
}

export function debugAIChatTimeline(label: string, data: Record<string, unknown>) {
  if (!isAIChatTimelineDebugEnabled()) return;
  try {
    type DebugLog = {
      at: string;
      label: string;
      data: Record<string, unknown>;
    };
    let debugElement = document.getElementById(AICHAT_TIMELINE_DEBUG_ELEMENT_ID) as HTMLScriptElement | null;
    if (!debugElement) {
      debugElement = document.createElement('script');
      debugElement.id = AICHAT_TIMELINE_DEBUG_ELEMENT_ID;
      debugElement.type = 'application/json';
      debugElement.textContent = '[]';
      document.documentElement.appendChild(debugElement);
    }
    let logs: DebugLog[] = [];
    try {
      logs = JSON.parse(debugElement.textContent || '[]') as DebugLog[];
    } catch {
      logs = [];
    }
    logs.push({
      at: new Date().toISOString(),
      label,
      data,
    });
    if (logs.length > AICHAT_TIMELINE_DEBUG_MAX_RECORDS) {
      logs.splice(0, logs.length - AICHAT_TIMELINE_DEBUG_MAX_RECORDS);
    }
    debugElement.textContent = JSON.stringify(logs);
  } catch {
    // Debug-only buffer; console output below is still useful if buffering fails.
  }
  console.debug(`[aichat-timeline] ${label}`, data);
}

function debugRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

export function summarizeAIChatTimeline(timeline: AIChatAgenticTimelineItem[]) {
  return timeline.map((item, index) => {
    if (item.type === 'skill_event') {
      const invocationRecord = debugRecord(item.invocation);
      return {
        index,
        type: item.type,
        kind: item.invocation.kind,
        runtime_id: item.invocation.runtime_id,
        event_id: item.event_id,
        created_at: item.created_at,
        created_at_ms: invocationRecord.created_at_ms,
        skill_id: item.invocation.skill_id,
        tool_name: item.invocation.tool_name,
        status: item.invocation.status,
      };
    }
    if (item.type === 'progress_text') {
      return {
        index,
        type: item.type,
        phase: item.phase,
        event_id: item.event_id,
        created_at: item.created_at,
        content_len: item.content.length,
        content_preview: item.content.slice(0, 80),
        skill_id: item.skill_id,
        tool_name: item.tool_name,
      };
    }
    if (item.type === 'intermediate_answer') {
      return {
        index,
        type: item.type,
        answer_id: item.answer_id,
        created_at: item.created_at,
        content_len: item.content.length,
        status: item.status,
      };
    }
    if (item.type === 'tool_governance_decision') {
      return {
        index,
        type: item.type,
        event_id: item.event_id,
        created_at: item.created_at,
        correlation_id: item.event.correlation_id,
        skill_id: item.event.skill_id,
        tool_name: item.event.tool_name,
        status: item.event.status,
        approval_status: item.event.approval_status,
      };
    }
    return {
      index,
      type: item.type,
    };
  });
}
