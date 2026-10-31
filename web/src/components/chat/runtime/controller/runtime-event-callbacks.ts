import type { AIChatMemoryMutationEventData } from '@/services/types/aichat';
import type { AIChatStreamCallbacks } from '@/components/chat/transports/aichat-transport';
import { isStaleAIChatStreamEvent } from '@/components/chat/controllers/aichat/reducers/shared';
import type { ChatRuntimeEventAppliers } from './use-chat-runtime-event-appliers';

/**
 * Callbacks that shape the stream's lifecycle. Each entry point (send,
 * regenerate, continuation, recovery) keeps its own, because they track the
 * stream they opened.
 */
type RuntimeLifecycleCallbackName =
  | 'onOpen'
  | 'onMessageStart'
  | 'onMessageEnd'
  | 'onErrorEvent'
  | 'onRequestError'
  | 'onClose';

/** Every other stream event only applies a fact to the controller state. */
export type RuntimeEventCallbackName = Exclude<
  keyof AIChatStreamCallbacks,
  RuntimeLifecycleCallbackName
>;

/**
 * One handler per event, none optional: an event added to the transport
 * without a handler here is a type error, not an event every entry point
 * silently drops (ADR 0050).
 */
export type RuntimeEventCallbacks = {
  [Name in RuntimeEventCallbackName]-?: NonNullable<AIChatStreamCallbacks[Name]>;
};

interface RuntimeEventCallbackOptions {
  appliers: ChatRuntimeEventAppliers;
  refreshAccountMemoryAfterMemoryMutation: (payload: AIChatMemoryMutationEventData) => void;
  /** The stream was abandoned; its remaining events change nothing. */
  isAborted: () => boolean;
  /** Called before each event is applied, for entry points that track whether the stream started. */
  onEventApplied?: () => void;
}

/**
 * Builds the event callbacks every stream entry point shares from the
 * controller's appliers. The entry point adds its lifecycle callbacks.
 *
 * The callbacks belong to one stream and keep its cursor: an event whose id
 * is not newer than the last one applied from this stream is a repeat and
 * changes nothing (ADR 0050). Events without an id are applied.
 */
export function createRuntimeEventCallbacks({
  appliers,
  refreshAccountMemoryAfterMemoryMutation,
  isAborted,
  onEventApplied,
}: RuntimeEventCallbackOptions): RuntimeEventCallbacks {
  const handlers: RuntimeEventCallbacks = {
    onAgentProgress: appliers.applyAgentProgress,
    onPlanUpdated: appliers.applyPlanUpdated,
    onIntermediateAnswer: appliers.applyIntermediateAnswer,
    onUserInputRequested: appliers.applyUserInputRequested,
    onFileParseStart: appliers.applyFileParseStart,
    onFileParseEnd: appliers.applyFileParseEnd,
    onFileParseError: appliers.applyFileParseError,
    onSkillLoadStart: appliers.applySkillLoadStart,
    onSkillLoadEnd: appliers.applySkillLoadEnd,
    onSkillReferenceRead: appliers.applySkillReferenceRead,
    onSkillCallStart: appliers.applySkillCallStart,
    onSkillCallEnd: appliers.applySkillCallEnd,
    onSkillCallError: appliers.applySkillCallError,
    onSkillArtifactCreated: appliers.applySkillArtifactCreated,
    onToolGovernanceDecision: appliers.applyToolGovernanceDecision,
    onClientActionRequired: appliers.applyClientActionRequired,
    onClientActionResult: appliers.applyClientActionResult,
    onMemoryMutation: (payload, eventId) => {
      appliers.applyMemoryMutation(payload, eventId);
      refreshAccountMemoryAfterMemoryMutation(payload);
    },
    onRuntimeInputApplied: appliers.applyRuntimeInputApplied,
    onWorkflowStarted: appliers.applyWorkflowStarted,
    onWorkflowNodeStarted: appliers.applyWorkflowNodeStarted,
    onWorkflowNodeFinished: appliers.applyWorkflowNodeFinished,
    onWorkflowPaused: appliers.applyWorkflowPaused,
    onWorkflowApprovalRequested: appliers.applyWorkflowApprovalRequested,
    onWorkflowFinished: appliers.applyWorkflowFinished,
    onWorkflowFailed: appliers.applyWorkflowFailed,
    onMessageChunk: appliers.applyMessageChunk,
    onMessageRetract: appliers.applyMessageRetract,
    onTokenUsageUpdated: appliers.applyTokenUsageUpdated,
    onTokenBudgetRequested: appliers.applyTokenBudgetRequested,
  };
  const guarded = {} as Record<
    RuntimeEventCallbackName,
    (payload: never, eventId?: string | null) => void
  >;
  let cursor: string | null = null;
  for (const name of Object.keys(handlers) as RuntimeEventCallbackName[]) {
    const handle = handlers[name] as (payload: never, eventId?: string | null) => void;
    guarded[name] = (payload, eventId) => {
      if (isAborted()) return;
      if (eventId) {
        if (isStaleAIChatStreamEvent(eventId, cursor)) return;
        cursor = eventId;
      }
      onEventApplied?.();
      handle(payload, eventId);
    };
  }
  return guarded as unknown as RuntimeEventCallbacks;
}
