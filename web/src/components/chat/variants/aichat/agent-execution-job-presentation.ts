import type { AIChatSkillInvocation } from '@/services/types/aichat';

type JobLabelKey =
  | 'consoleChat.skills.agentic.jobSucceeded'
  | 'consoleChat.skills.agentic.jobFailed'
  | 'consoleChat.skills.agentic.jobRunning'
  | 'consoleChat.skills.agentic.jobUnconfirmed';

/** Shell transport completion does not establish a successful process exit. */
export function getAgentExecutionJobPresentation(invocation: AIChatSkillInvocation): {
  tone: 'running' | 'success' | 'warning' | 'error';
  labelKey: JobLabelKey;
  exitCode: number | null;
} | null {
  if (invocation.skill_id !== 'agent_execution' || invocation.tool_name !== 'shell_exec') {
    return null;
  }
  const result = invocation.result;
  if (!result || typeof result.job_id !== 'string' || !result.job_id.trim()) return null;
  const status = typeof result.status === 'string' ? result.status : '';
  const exitCode =
    typeof result.exit_code === 'number' && Number.isSafeInteger(result.exit_code)
      ? result.exit_code
      : null;
  if (
    status === 'failed' ||
    status === 'canceled' ||
    status === 'cancelled' ||
    status === 'timed_out' ||
    (exitCode !== null && exitCode !== 0)
  ) {
    return { tone: 'error', labelKey: 'consoleChat.skills.agentic.jobFailed', exitCode };
  }
  if (invocation.status === 'error') {
    return { tone: 'error', labelKey: 'consoleChat.skills.agentic.jobUnconfirmed', exitCode };
  }
  if (status === 'queued' || status === 'running') {
    return { tone: 'running', labelKey: 'consoleChat.skills.agentic.jobRunning', exitCode };
  }
  if (status === 'succeeded' && exitCode === 0) {
    return { tone: 'success', labelKey: 'consoleChat.skills.agentic.jobSucceeded', exitCode };
  }
  return { tone: 'warning', labelKey: 'consoleChat.skills.agentic.jobUnconfirmed', exitCode };
}
