'use client';

import { createElement, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import {
  WorkflowBillingToastAction,
  workflowBillingToastClassNames,
} from '@/components/workflow/common/workflow-billing-toast-action';
import { useWorkspaceStore } from '@/store/workspace-store';
import { useAccountCapabilities } from '@/hooks/use-account-capabilities';
import type { WorkflowRunBillingError, WorkflowPrecheckWarning } from '@/services/types/workflow';
import {
  extractWorkflowRunError,
  getWorkflowBillingErrorMessage,
  getWorkflowPrecheckWarningMessage,
  isWorkflowBillingErrorCode,
  resolveWorkflowBillingErrorCode,
  sortWorkflowPrecheckWarnings,
  type WorkflowBillingTranslationScope,
  type WorkflowTranslator,
} from '@/utils/workflow/billing';
import { classifyWorkflowRuntimeError } from '@/utils/workflow/runtime-error';

interface WorkflowPrecheckWarningView {
  code?: string;
  title: string;
  description: string;
}

interface UseWorkflowBillingFeedbackReturn {
  getWorkflowRunErrorText: (error: unknown) => string | undefined;
  notifyBillingError: (error: unknown) => boolean;
  getPrecheckWarningViews: (warnings: WorkflowPrecheckWarning[]) => WorkflowPrecheckWarningView[];
  parseWorkflowRunError: (error: unknown) => WorkflowRunBillingError | null;
}

/**
 * @hook useWorkflowBillingFeedback
 * @description Maps workflow billing/precheck codes into localized UI copy and CTA toasts.
 */
export function useWorkflowBillingFeedback(
  scope: WorkflowBillingTranslationScope
): UseWorkflowBillingFeedbackReturn {
  const router = useRouter();
  const t = useT();
  const connectorT = useT('connectors');
  const billingT = useCallback<WorkflowTranslator>((key, values) => t(key as never, values), [t]);
  const connectorTranslator = useCallback<WorkflowTranslator>(
    (key, values) => connectorT(key as never, values),
    [connectorT]
  );
  const currentWorkspace = useWorkspaceStore.use.currentWorkspace();
  const { canManageModelConfig: isAdmin } = useAccountCapabilities();

  const parseWorkflowRunError = useCallback((error: unknown) => extractWorkflowRunError(error), []);

  const getWorkflowRunErrorText = useCallback(
    (error: unknown) => {
      const parsed = extractWorkflowRunError(error);
      const message = getWorkflowBillingErrorMessage(billingT, scope, parsed, {
        isAdmin,
        workspaceId: currentWorkspace?.id,
        includeRuntimeDetails: true,
        connectorTranslator,
      });
      return message?.description;
    },
    [billingT, currentWorkspace?.id, connectorTranslator, isAdmin, scope]
  );

  const notifyBillingError = useCallback(
    (error: unknown) => {
      const parsed = extractWorkflowRunError(error);
      const message = getWorkflowBillingErrorMessage(billingT, scope, parsed, {
        isAdmin,
        workspaceId: currentWorkspace?.id,
        connectorTranslator,
      });

      if (!message) {
        return false;
      }

      const code = resolveWorkflowBillingErrorCode(parsed?.code, parsed?.message);
      const runtimeErrorKind = classifyWorkflowRuntimeError(parsed?.message);
      const connectorCode =
        [parsed?.code, parsed?.message].find(value => typeof value === 'string' && /^connector_[a-z_]+$/.test(value));
      const isBillingError = isWorkflowBillingErrorCode(code);
      const toastFn = isBillingError ? toast.warning : toast.error;

      toastFn(message.title, {
        id: connectorCode
          ? `workflow-connector-${scope}-${connectorCode}`
          : code
            ? `workflow-billing-${scope}-${code}`
            : runtimeErrorKind
              ? `workflow-runtime-${scope}-${runtimeErrorKind}`
              : undefined,
        description: message.description,
        classNames: isBillingError ? workflowBillingToastClassNames : undefined,
        action:
          isAdmin && message.href && message.actionLabel
            ? createElement(WorkflowBillingToastAction, {
                label: message.actionLabel,
                onClick: () => router.push(message.href as string),
              })
            : undefined,
      });
      return true;
    },
    [billingT, currentWorkspace?.id, connectorTranslator, isAdmin, router, scope]
  );

  const getPrecheckWarningViews = useCallback(
    (warnings: WorkflowPrecheckWarning[]) =>
      sortWorkflowPrecheckWarnings(warnings).map(warning =>
        getWorkflowPrecheckWarningMessage(billingT, scope, warning)
      ),
    [billingT, scope]
  );

  return {
    getWorkflowRunErrorText,
    notifyBillingError,
    getPrecheckWarningViews,
    parseWorkflowRunError,
  };
}
