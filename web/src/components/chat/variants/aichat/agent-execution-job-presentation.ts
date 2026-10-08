import type { AIChatSkillInvocation } from '@/services/types/aichat';

type JobLabelKey =
  | 'consoleChat.skills.agentic.jobSucceeded'
  | 'consoleChat.skills.agentic.jobFailed'
  | 'consoleChat.skills.agentic.jobRunning'
  | 'consoleChat.skills.agentic.jobCanceled'
  | 'consoleChat.skills.agentic.jobTimedOut'
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
  if (status === 'canceled' || status === 'cancelled') {
    return { tone: 'warning', labelKey: 'consoleChat.skills.agentic.jobCanceled', exitCode };
  }
  if (status === 'timed_out') {
    return { tone: 'error', labelKey: 'consoleChat.skills.agentic.jobTimedOut', exitCode };
  }
  if (status === 'failed' || (exitCode !== null && exitCode !== 0)) {
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

/** Old traces have a default zero duration, not a measured sub-millisecond Job. */
export function agentExecutionDuration(invocation: AIChatSkillInvocation): number | undefined {
  const duration = invocation.duration_ms;
  return typeof duration === 'number' && Number.isFinite(duration) && duration > 0
    ? duration
    : undefined;
}

const runtimeLabelKeys = {
  python: 'consoleChat.skills.agentic.executionPython',
  node: 'consoleChat.skills.agentic.executionNode',
  package_manager: 'consoleChat.skills.agentic.executionPackages',
  shell: 'consoleChat.skills.agentic.executionShell',
} as const;
const diagnosticLabelKeys = {
  missing_python_dependency: 'consoleChat.skills.agentic.missingPythonDependency',
  missing_node_dependency: 'consoleChat.skills.agentic.missingNodeDependency',
  command_not_found: 'consoleChat.skills.agentic.commandNotFound',
} as const;

/** Whitelist display enums. Never fall back to arbitrary result strings. */
export function getAgentExecutionDisplay(invocation: AIChatSkillInvocation) {
  const value = invocation.result?.execution_display;
  const display =
    value && typeof value === 'object' && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : {};
  const runtime =
    typeof display.runtime === 'string' && Object.hasOwn(runtimeLabelKeys, display.runtime)
      ? (display.runtime as keyof typeof runtimeLabelKeys)
      : 'shell';
  const diagnostic =
    typeof display.diagnostic === 'string' && Object.hasOwn(diagnosticLabelKeys, display.diagnostic)
      ? diagnosticLabelKeys[display.diagnostic as keyof typeof diagnosticLabelKeys]
      : null;
  return {
    runtimeLabelKey: runtimeLabelKeys[runtime],
    diagnosticLabelKey: diagnostic,
    hasOutput: display.has_stdout === true || display.has_stderr === true,
  };
}
