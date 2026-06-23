'use client';

import { use, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { AlertCircle, Loader2, RefreshCcw } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { useAgent } from '@/hooks/agent/use-agents';
import { useT } from '@/i18n';
import { AgentType } from '@/services/types/agent';
import { getErrorMessage } from '@/utils/error-notifications';

interface AgentEntryPageProps {
  params: Promise<{
    agentId: string;
  }>;
}

function getAgentDefaultHref(agentId: string, agentType?: AgentType | string) {
  const normalizedAgentType = String(agentType ?? '').toUpperCase();

  if (
    normalizedAgentType === AgentType.WORKFLOW ||
    normalizedAgentType === AgentType.CONVERSATIONAL_AGENT
  ) {
    return `/console/agents/${agentId}/workflow`;
  }

  return `/console/agents/${agentId}/agent`;
}

export default function AgentEntryPage({ params }: AgentEntryPageProps) {
  const t = useT();
  const router = useRouter();
  const { agentId } = use(params);
  const { agent, isLoading, error, refetch } = useAgent(agentId);

  useEffect(() => {
    if (!agent?.data) {
      return;
    }

    router.replace(getAgentDefaultHref(agentId, agent.data.agent_type));
  }, [agent?.data, agentId, router]);

  if (isLoading || agent?.data) {
    return (
      <div className="flex h-full w-full items-center justify-center p-6">
        <Loader2 className="size-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  const message = error ? getErrorMessage(error) : '';

  return (
    <div className="flex h-full w-full items-center justify-center p-6">
      <div className="w-full max-w-xl">
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>
            {error ? t('agents.workflow.loadFailedTitle') : t('agents.workflow.notFoundTitle')}
          </AlertTitle>
          <AlertDescription>
            {error
              ? message || t('agents.workflow.loadFailedDesc')
              : t('agents.workflow.notFoundDesc')}
          </AlertDescription>
        </Alert>
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
      </div>
    </div>
  );
}
