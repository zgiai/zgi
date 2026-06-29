'use client';

import { use, useEffect } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { AlertCircle, Loader2, RefreshCcw } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { useAgent } from '@/hooks/agent/use-agents';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { AGENT_PERMISSION_ACTIONS, WORKFLOW_PERMISSION_ACTIONS } from '@/constants/permissions';
import {
  getAgentDetailDefaultHref,
  isAgentRuntimeType,
  isWorkflowRuntimeType,
} from '@/utils/agent-detail-routes';
import { getErrorMessage } from '@/utils/error-notifications';

interface AgentEntryPageProps {
  params: Promise<{
    agentId: string;
  }>;
}

export default function AgentEntryPage({ params }: AgentEntryPageProps) {
  const t = useT();
  const router = useRouter();
  const searchParams = useSearchParams();
  const { agentId } = use(params);
  const { agent, isLoading, error, refetch } = useAgent(agentId);
  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const agentType = agent?.data?.agent_type;
  const isAgentRuntime = isAgentRuntimeType(agentType);
  const isWorkflowRuntime = isWorkflowRuntimeType(agentType);
  const canCreateAgent = hasAnyPermission(AGENT_PERMISSION_ACTIONS.create);
  const canImportAgent = hasAnyPermission(AGENT_PERMISSION_ACTIONS.import);
  const canUpdateAgent = hasAnyPermission(AGENT_PERMISSION_ACTIONS.update);
  const canOpenAgentRuntimeEditor =
    isAgentRuntime &&
    (canCreateAgent ||
      canImportAgent ||
      canUpdateAgent ||
      hasAnyPermission(AGENT_PERMISSION_ACTIONS.runtimeConfigManage) ||
      hasAnyPermission(AGENT_PERMISSION_ACTIONS.publish) ||
      hasAnyPermission(AGENT_PERMISSION_ACTIONS.runtimeAccessManage));
  const canCreateWorkflow = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.create);
  const canImportWorkflow = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.import);
  const canOpenWorkflowEditor =
    isWorkflowRuntime &&
    (canCreateWorkflow ||
      canImportWorkflow ||
      hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.update) ||
      hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.runDraft) ||
      hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.runStop) ||
      hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.debug) ||
      hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.publish) ||
      hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.runtimeAccessManage));
  const canViewAgentLogs = hasAnyPermission(AGENT_PERMISSION_ACTIONS.logsView);
  const canManageWorkflowRuntimeAccess = hasAnyPermission(
    WORKFLOW_PERMISSION_ACTIONS.runtimeAccessManage
  );
  const canViewWorkflowLogs = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.logsView);
  const canViewWorkflowTestLibrary = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.view);
  const canRunWorkflowBatchTest = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.debug);
  const targetHref = agent?.data
    ? getAgentDetailDefaultHref(agentId, agentType, {
        canView: true,
        canOpenEditor: isAgentRuntime ? canOpenAgentRuntimeEditor : canOpenWorkflowEditor,
        canManageRuntimeAccess: isWorkflowRuntime && canManageWorkflowRuntimeAccess,
        canViewRuntimeLogs: isAgentRuntime ? canViewAgentLogs : canViewWorkflowLogs,
        canViewBatchTest:
          isWorkflowRuntime &&
          (canViewWorkflowTestLibrary || canViewWorkflowLogs || canRunWorkflowBatchTest),
        canRunBatchTest: isWorkflowRuntime && canRunWorkflowBatchTest,
        isPublished: agent.data.is_published,
        preferBatchTestLibrary: canViewWorkflowTestLibrary,
      })
    : null;
  const targetHrefWithSearch =
    targetHref && searchParams.toString() ? `${targetHref}?${searchParams.toString()}` : targetHref;

  useEffect(() => {
    if (!targetHrefWithSearch) {
      return;
    }

    router.replace(targetHrefWithSearch);
  }, [router, targetHrefWithSearch]);

  if (isLoading || isPermissionsLoading || targetHref) {
    return (
      <div className="flex h-full w-full items-center justify-center p-6">
        <Loader2 className="size-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  const message = error ? getErrorMessage(error) : '';
  const isAccessDenied = agent?.data && !targetHref;

  return (
    <div className="flex h-full w-full items-center justify-center p-6">
      <div className="w-full max-w-xl">
        <Alert variant={error ? 'destructive' : 'default'}>
          <AlertCircle className="size-4" />
          <AlertTitle>
            {isAccessDenied
              ? t('common.accessDenied')
              : error
                ? t('agents.workflow.loadFailedTitle')
                : t('agents.workflow.notFoundTitle')}
          </AlertTitle>
          <AlertDescription>
            {isAccessDenied
              ? t('common.unauthorizedDescription')
              : error
                ? message || t('agents.workflow.loadFailedDesc')
                : t('agents.workflow.notFoundDesc')}
          </AlertDescription>
        </Alert>
        {error ? (
          <div className="mt-4 flex gap-2">
            <Button
              variant="default"
              onClick={() => {
                void refetch();
              }}
            >
              <RefreshCcw className="mr-2 size-4" />
              {t('agents.actions.retry')}
            </Button>
          </div>
        ) : null}
      </div>
    </div>
  );
}
