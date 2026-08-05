import type { AIChatAgentProgressEventData } from '@/services/types/aichat';

export type AIChatModelProcessingStage = 'initial' | 'extended' | 'long_running';
export type AIChatModelProcessingActivity =
  | 'awaiting_response'
  | 'reviewing_tool_result'
  | 'reasoning'
  | 'preparing_action';
export type AIChatModelProcessingSource = 'runtime' | 'provider_signal';

export interface AIChatModelProcessingState {
  progress_id: string;
  stage: AIChatModelProcessingStage;
  activity: AIChatModelProcessingActivity;
  source?: AIChatModelProcessingSource;
  round?: number;
  elapsed_ms?: number;
  created_at?: number;
  event_id?: string | null;
}

const MODEL_PROCESSING_STAGE_RANK: Record<AIChatModelProcessingStage, number> = {
  initial: 0,
  extended: 1,
  long_running: 2,
};

const MODEL_PROCESSING_ACTIVITY_RANK: Record<AIChatModelProcessingActivity, number> = {
  awaiting_response: 0,
  reviewing_tool_result: 1,
  reasoning: 2,
  preparing_action: 3,
};

function isModelProcessingStage(value: unknown): value is AIChatModelProcessingStage {
  return value === 'initial' || value === 'extended' || value === 'long_running';
}

function modelProcessingActivity(value: unknown): AIChatModelProcessingActivity {
  switch (value) {
    case 'reviewing_tool_result':
    case 'reasoning':
    case 'preparing_action':
      return value;
    default:
      return 'awaiting_response';
  }
}

function modelProcessingSource(value: unknown): AIChatModelProcessingSource | undefined {
  return value === 'runtime' || value === 'provider_signal' ? value : undefined;
}

export function modelProcessingStateFromEvent(
  current: AIChatModelProcessingState | undefined,
  payload: AIChatAgentProgressEventData,
  eventId?: string | null
): AIChatModelProcessingState | undefined {
  if (
    payload.phase !== 'model_processing' ||
    !payload.progress_id?.trim() ||
    !isModelProcessingStage(payload.stage)
  ) {
    return current;
  }
  if (eventId && current?.event_id === eventId) {
    return current;
  }
  const activity = modelProcessingActivity(payload.activity);
  if (
    current?.progress_id === payload.progress_id &&
    (MODEL_PROCESSING_STAGE_RANK[payload.stage] < MODEL_PROCESSING_STAGE_RANK[current.stage] ||
      MODEL_PROCESSING_ACTIVITY_RANK[activity] < MODEL_PROCESSING_ACTIVITY_RANK[current.activity])
  ) {
    return current;
  }
  return {
    progress_id: payload.progress_id.trim(),
    stage: payload.stage,
    activity,
    source: modelProcessingSource(payload.source),
    round: payload.round,
    elapsed_ms: payload.elapsed_ms,
    created_at: payload.created_at,
    event_id: eventId ?? null,
  };
}
