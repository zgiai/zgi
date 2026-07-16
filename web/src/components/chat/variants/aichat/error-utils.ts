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
  const billingMessage = getWorkflowBillingErrorMessage(t, 'webapp', parsed, options);
  const code = resolveWorkflowBillingErrorCode(parsed.code, parsed.message);

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
