import type {
  AIChatMessageFile,
  AIChatMessageMetadata,
  AIChatSkillInvocation,
} from '@/services/types/aichat';
import { type AIChatAgenticTimelineItem } from '@/components/chat/controllers/aichat/types';

export function createAIChatFileMetadata(
  files?: AIChatMessageFile[]
): AIChatMessageMetadata | undefined {
  if (!files?.length) {
    return undefined;
  }

  return {
    file_count: files.length,
    files,
  };
}

export function mergeMessageMetadata(
  existingMetadata?: AIChatMessageMetadata,
  incomingMetadata?: AIChatMessageMetadata
): AIChatMessageMetadata | undefined {
  if (!existingMetadata && !incomingMetadata) {
    return undefined;
  }

  const existingFiles = existingMetadata?.files ?? [];
  const incomingFiles = incomingMetadata?.files ?? [];
  const files = incomingFiles.length > 0 ? incomingFiles : existingFiles;
  const existingGeneratedFiles = existingMetadata?.generated_files ?? [];
  const incomingGeneratedFiles = incomingMetadata?.generated_files ?? [];
  const generatedFiles =
    incomingGeneratedFiles.length > 0 ? incomingGeneratedFiles : existingGeneratedFiles;
  const existingWorkflowRuns = existingMetadata?.workflow_runs ?? [];
  const incomingWorkflowRuns = incomingMetadata?.workflow_runs ?? [];
  const workflowRuns =
    incomingWorkflowRuns.length > 0 ? incomingWorkflowRuns : existingWorkflowRuns;
  const userInputRequest =
    incomingMetadata?.user_input_request ?? existingMetadata?.user_input_request;
  const existingSkillInvocations = visibleSkillInvocations(existingMetadata?.skill_invocations);
  const incomingSkillInvocations = visibleSkillInvocations(incomingMetadata?.skill_invocations);
  const hasSkillInvocationMetadata = Boolean(
    existingMetadata?.skill_invocations || incomingMetadata?.skill_invocations
  );
  const skillInvocations =
    incomingSkillInvocations.length > 0 ? incomingSkillInvocations : existingSkillInvocations;
  const loadedSkillIds = uniqueStrings(
    skillInvocations
      .filter(item => item.kind === 'skill_load' && item.status !== 'error')
      .map(item => item.skill_id)
  );
  const skillNames = uniqueStrings(skillInvocations.map(item => item.skill_id));
  const toolNames = uniqueStrings(
    skillInvocations
      .filter(item => item.kind === 'tool_call')
      .map(item => item.tool_name)
      .filter((toolName): toolName is string => Boolean(toolName))
  );

  return {
    ...(existingMetadata ?? {}),
    ...(incomingMetadata ?? {}),
    ...(files.length > 0
      ? {
          file_count: files.length,
          files,
        }
      : {}),
    ...(generatedFiles.length > 0
      ? {
          generated_file_count: generatedFiles.length,
          generated_files: generatedFiles,
        }
      : {}),
    ...(workflowRuns.length > 0
      ? {
          workflow_run_count: workflowRuns.length,
          workflow_runs: workflowRuns,
        }
      : {}),
    ...(hasSkillInvocationMetadata
      ? {
          has_trace: skillInvocations.length > 0,
          skill_invocations: skillInvocations,
          selected_skill_ids: skillNames,
          loaded_skill_ids: loadedSkillIds,
          skill_step_count: skillInvocations.length,
          skill_call_count: skillInvocations.length,
          skill_names: skillNames,
          tool_call_count: skillInvocations.filter(item => item.kind === 'tool_call').length,
          tool_names: toolNames,
        }
      : {}),
    ...(userInputRequest
      ? {
          user_input_request: userInputRequest,
        }
      : {}),
  };
}

function visibleSkillInvocations(
  invocations: AIChatSkillInvocation[] | undefined
): AIChatSkillInvocation[] {
  return (invocations ?? []).filter(invocation => {
    const status = String(invocation.status ?? '').toLowerCase();
    if (
      invocation.kind === 'client_action' &&
      invocation.action_type === 'route_navigation' &&
      (status === 'success' || status === 'succeeded')
    ) {
      return false;
    }
    return invocation.kind !== 'metadata_exposed' && invocation.kind !== 'memory_planner';
  });
}

function uniqueStrings(values: Array<string | undefined>): string[] {
  return Array.from(
    new Set(values.map(value => value?.trim()).filter((value): value is string => Boolean(value)))
  );
}

export function clearRuntimeMessageMetadata(
  metadata?: AIChatMessageMetadata
): AIChatMessageMetadata | undefined {
  if (!metadata) return undefined;
  const next = { ...metadata };
  delete next.sensitiveOutputBlocked;
  delete next.has_trace;
  delete next.skill_invocations;
  delete next.selected_skill_ids;
  delete next.loaded_skill_ids;
  delete next.skill_step_count;
  delete next.skill_call_count;
  delete next.skill_names;
  delete next.tool_call_count;
  delete next.tool_names;
  delete next.generated_file_count;
  delete next.generated_files;
  delete next.workflow_run_count;
  delete next.workflow_runs;
  delete next.user_input_request;
  return next;
}

export function isTransientProgressItem(item: AIChatAgenticTimelineItem): boolean {
  return (
    item.type === 'progress_text' &&
    (item.transient === true || Boolean(item.phase && !item.content.trim()))
  );
}

export function removeTransientProgressItems(
  timeline: AIChatAgenticTimelineItem[] | undefined
): AIChatAgenticTimelineItem[] {
  return (timeline ?? []).filter(item => !isTransientProgressItem(item));
}
