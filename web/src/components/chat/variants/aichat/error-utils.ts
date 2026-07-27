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

function includesAny(value: string, candidates: string[]): boolean {
  return candidates.some(candidate => value.includes(candidate));
}

function resolveFriendlyRuntimeError(
  t: WorkflowTranslator,
  rawMessage: string | undefined,
  code: string | undefined,
  isAdmin: boolean
): Pick<AIChatErrorDisplayMessage, 'title' | 'description'> | null {
  const normalized = rawMessage?.toLowerCase() ?? '';

  if (
    code === '399001' ||
    includesAny(normalized, ['internal server error', 'unknown error', 'status code 500'])
  ) {
    return {
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
  const billingCode = resolveWorkflowBillingErrorCode(parsed.code, parsed.message);
  const code =
    billingCode ??
    (typeof parsed.code === 'string' || typeof parsed.code === 'number'
      ? String(parsed.code).trim() || undefined
      : undefined);
  const friendlyRuntimeError = resolveFriendlyRuntimeError(
    t,
    rawMessage,
    code,
    Boolean(options.isAdmin)
  );

  return {
    code,
    title: billingMessage?.title || friendlyRuntimeError?.title,
    description:
      billingMessage?.description || friendlyRuntimeError?.description || fallbackDescription,
    actionLabel: billingMessage?.actionLabel,
    href: billingMessage?.href,
    isBilling: isWorkflowBillingErrorCode(billingCode),
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
