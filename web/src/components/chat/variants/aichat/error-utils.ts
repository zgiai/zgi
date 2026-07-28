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
}

type AIChatRuntimeErrorCode = WorkflowRuntimeErrorKind | 'agent_final_answer_unavailable';

function resolveAIChatRuntimeErrorCode(
  code: string | number | undefined,
  message: string | undefined
): AIChatRuntimeErrorCode | undefined {
  const normalizedCode = typeof code === 'string' ? code.trim().toLowerCase() : '';
  if (normalizedCode === 'agent_final_answer_unavailable') {
    return 'agent_final_answer_unavailable';
  }
  if (
    normalizedCode === 'model_service_timeout' ||
    normalizedCode === 'model_service_unavailable' ||
    normalizedCode === 'model_invocation_failed'
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
    };
  }

  const parsed: WorkflowRunBillingError = {
    code: input?.code,
    message: rawMessage,
    params: input?.params,
  };
  const code = resolveWorkflowBillingErrorCode(parsed.code, parsed.message);
  if (!isWorkflowBillingErrorCode(code)) {
    const runtimeErrorCode = resolveAIChatRuntimeErrorCode(input?.code, rawMessage);
    if (runtimeErrorCode) {
      return {
        code: runtimeErrorCode,
        description: t(`agents.workflow.errors.${runtimeErrorCode}`),
        isBilling: false,
      };
    }
  }

  const billingMessage = getWorkflowBillingErrorMessage(t, 'webapp', parsed, options);

  return {
    code,
    title: billingMessage?.title,
    description: billingMessage?.description || fallbackDescription,
    actionLabel: billingMessage?.actionLabel,
    href: billingMessage?.href,
    isBilling: isWorkflowBillingErrorCode(code),
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
