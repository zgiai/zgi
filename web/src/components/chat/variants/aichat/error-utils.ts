import type { AIChatMessage } from '@/services/types/aichat';
import type { WorkflowRunBillingError } from '@/services/types/workflow';
import { isContinuationLikelyStartedError } from '@/components/chat/runtime/controller/chat-runtime-controller-utils';
import {
  getWorkflowBillingErrorMessage,
  isWorkflowBillingErrorCode,
  resolveWorkflowBillingErrorCode,
  type WorkflowBillingMessageOptions,
  type WorkflowTranslator,
} from '@/utils/workflow/billing';
import {
  classifyWorkflowRuntimeError,
  type WorkflowRuntimeErrorKind,
} from '@/utils/workflow/runtime-error';

export interface AIChatErrorDisplayInput {
  code?: string | number;
  message?: string;
  params?: Record<string, unknown>;
}

export interface AIChatErrorDisplayMessage {
  code?: string;
  title?: string;
  description: string;
  actionLabel?: string;
  href?: string | null;
  isBilling: boolean;
  kind: 'billing' | 'server' | 'timeout' | 'network' | 'provider' | 'unknown';
}

function includesAny(value: string, candidates: string[]): boolean {
  return candidates.some(candidate => value.includes(candidate));
}

function resolveFriendlyRuntimeError(
  t: WorkflowTranslator,
  rawMessage: string | undefined,
  code: string | undefined,
  isAdmin: boolean
): Pick<AIChatErrorDisplayMessage, 'title' | 'description' | 'kind'> | null {
  const normalized = rawMessage?.toLowerCase() ?? '';

  if (
    code === '399001' ||
    includesAny(normalized, ['internal server error', 'unknown error', 'status code 500'])
  ) {
    return {
      kind: 'server',
      title: t('webapp.consoleChat.errors.server.title'),
      description: t(
        isAdmin
          ? 'webapp.consoleChat.errors.server.adminDescription'
          : 'webapp.consoleChat.errors.server.memberDescription'
      ),
    };
  }

  if (
    includesAny(normalized, [
      'timeout',
      'timed out',
      'deadline exceeded',
      '长时间未返回',
      '响应超时',
    ])
  ) {
    return {
      kind: 'timeout',
      title: t('webapp.consoleChat.errors.timeout.title'),
      description: t('webapp.consoleChat.errors.timeout.description'),
    };
  }

  if (
    includesAny(normalized, [
      'network error',
      'failed to fetch',
      'connection reset',
      'connection refused',
      'stream disconnected',
      '网络',
      '连接已断开',
    ])
  ) {
    return {
      kind: 'network',
      title: t('webapp.consoleChat.errors.network.title'),
      description: t('webapp.consoleChat.errors.network.description'),
    };
  }

  if (
    includesAny(normalized, [
      'all providers failed',
      'no available provider',
      'provider unavailable',
      'upstream service error',
      'channel unavailable',
      '渠道不可用',
      '上游服务',
    ])
  ) {
    return {
      kind: 'provider',
      title: t('webapp.consoleChat.errors.provider.title'),
      description: t(
        isAdmin
          ? 'webapp.consoleChat.errors.provider.adminDescription'
          : 'webapp.consoleChat.errors.provider.memberDescription'
      ),
    };
  }

  return null;
}

type AIChatRuntimeErrorCode =
  | WorkflowRuntimeErrorKind
  | 'agent_final_answer_unavailable'
  | 'aichat.context.compaction_unavailable';

function resolveAIChatRuntimeErrorCode(
  code: string | number | undefined,
  message: string | undefined
): AIChatRuntimeErrorCode | undefined {
  const normalizedCode = typeof code === 'string' ? code.trim().toLowerCase() : '';
  if (normalizedCode === 'aichat.context.compaction_unavailable') {
    return 'aichat.context.compaction_unavailable';
  }
  if (normalizedCode === 'agent_final_answer_unavailable') {
    return 'agent_final_answer_unavailable';
  }
  if (
    normalizedCode === 'model_service_timeout' ||
    normalizedCode === 'model_service_unavailable' ||
    normalizedCode === 'model_invocation_failed' ||
    normalizedCode === 'planning_output_truncated' ||
    normalizedCode === 'agent_output_truncated' ||
    normalizedCode === 'server_unavailable'
  ) {
    return normalizedCode;
  }

  const normalizedMessage = message?.trim().toLowerCase();
  if (
    normalizedMessage?.includes('could not generate a usable final response after retrying') ||
    normalizedMessage?.includes('final answer unavailable')
  ) {
    return 'agent_final_answer_unavailable';
  }
  if (
    normalizedMessage?.includes('model did not respond before the task timed out') ||
    normalizedMessage?.includes('模型长时间未返回')
  ) {
    return 'model_service_timeout';
  }
  if (normalizedMessage?.includes('model service is temporarily unavailable')) {
    return 'model_service_unavailable';
  }
  if (normalizedMessage?.includes('model could not complete the requested response')) {
    return 'model_invocation_failed';
  }
  return classifyWorkflowRuntimeError(message);
}

function resolveStructuredRuntimeError(
  t: WorkflowTranslator,
  code: AIChatRuntimeErrorCode,
  isAdmin: boolean
): Pick<AIChatErrorDisplayMessage, 'title' | 'description' | 'kind'> {
  switch (code) {
    case 'aichat.context.compaction_unavailable':
      return {
        kind: 'server',
        title: t('webapp.consoleChat.contextCompactionBlocked.title'),
        description: t('webapp.consoleChat.contextCompactionBlocked.description'),
      };
    case 'model_service_timeout':
      return {
        kind: 'timeout',
        title: t('webapp.consoleChat.errors.timeout.title'),
        description: t(`agents.workflow.errors.${code}`),
      };
    case 'model_service_unavailable':
      return {
        kind: 'provider',
        title: t('webapp.consoleChat.errors.provider.title'),
        description: t(`agents.workflow.errors.${code}`),
      };
    case 'model_invocation_failed':
    case 'agent_final_answer_unavailable':
      return {
        kind: 'provider',
        title: t('webapp.consoleChat.errors.server.title'),
        description: t(`agents.workflow.errors.${code}`),
      };
    case 'planning_output_truncated':
      return {
        kind: 'server',
        title: t('webapp.chat.workflowErrorTitles.planning_output_truncated'),
        description: t('agents.workflow.errors.planning_output_truncated'),
      };
    case 'agent_output_truncated':
      return {
        kind: 'server',
        title: t('webapp.chat.workflowErrorTitles.agent_output_truncated'),
        description: t('agents.workflow.errors.agent_output_truncated'),
      };
    case 'server_unavailable':
      return {
        kind: 'server',
        title: t('webapp.consoleChat.errors.server.title'),
        description: t(
          isAdmin
            ? 'webapp.consoleChat.errors.server.adminDescription'
            : 'webapp.consoleChat.errors.server.memberDescription'
        ),
      };
    default:
      return {
        kind: 'unknown',
        title: t('webapp.consoleChat.errors.server.title'),
        description: t('agents.workflow.errors.executionFailed'),
      };
  }
}

export function resolveAIChatErrorMessage(
  t: WorkflowTranslator,
  input: AIChatErrorDisplayInput | null | undefined,
  options: WorkflowBillingMessageOptions = {}
): AIChatErrorDisplayMessage {
  const rawMessage = input?.message?.trim();
  const fallbackDescription = rawMessage || t('webapp.consoleChat.streamError');
  if (!input?.code && !rawMessage) {
    return {
      description: fallbackDescription,
      isBilling: false,
      kind: 'unknown',
    };
  }

  const parsed: WorkflowRunBillingError = {
    code: input?.code,
    message: rawMessage,
    params: input?.params,
  };
  const billingMessage = getWorkflowBillingErrorMessage(t, 'webapp', parsed, options);
  const billingCode = resolveWorkflowBillingErrorCode(parsed.code, parsed.message);
  const code =
    billingCode ??
    (typeof parsed.code === 'string' || typeof parsed.code === 'number'
      ? String(parsed.code).trim() || undefined
      : undefined);
  const isBilling = isWorkflowBillingErrorCode(billingCode);
  if (!isBilling) {
    const runtimeErrorCode = resolveAIChatRuntimeErrorCode(input?.code, rawMessage);
    if (runtimeErrorCode) {
      const runtimeError = resolveStructuredRuntimeError(
        t,
        runtimeErrorCode,
        Boolean(options.isAdmin)
      );
      return {
        code: runtimeErrorCode,
        title: runtimeError.title,
        description: runtimeError.description,
        isBilling: false,
        kind: runtimeError.kind,
      };
    }
  }

  const friendlyRuntimeError = resolveFriendlyRuntimeError(
    t,
    rawMessage,
    code,
    Boolean(options.isAdmin)
  );

  return {
    code,
    title: isBilling ? billingMessage?.title : friendlyRuntimeError?.title || billingMessage?.title,
    description:
      (isBilling
        ? billingMessage?.description
        : friendlyRuntimeError?.description || billingMessage?.description) || fallbackDescription,
    actionLabel: billingMessage?.actionLabel,
    href: billingMessage?.href,
    isBilling,
    kind: isBilling
      ? 'billing'
      : friendlyRuntimeError?.kind
        ? friendlyRuntimeError.kind
        : 'unknown',
  };
}

export function resolveAIChatErrorText(
  t: WorkflowTranslator,
  input: AIChatErrorDisplayInput | null | undefined,
  options: WorkflowBillingMessageOptions = {}
): string {
  return resolveAIChatErrorMessage(t, input, options).description;
}

export function getAIChatMessageErrorInput(message: AIChatMessage): AIChatErrorDisplayInput {
  return {
    code: message.metadata?.error_code as string | number | undefined,
    message: message.error,
    params: message.metadata?.error_params as Record<string, unknown> | undefined,
  };
}

export function isAIChatContinuationLikelyStarted(error: unknown): boolean {
  return isContinuationLikelyStartedError(error);
}
