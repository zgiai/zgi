'use client';

import { use } from 'react';
import {
  AgentRuntimeDialogs,
  AgentRuntimeLoadingState,
  AgentRuntimeWorkbench,
  useAgentRuntimePageModel,
} from '@/components/agents/agent-runtime';

interface AgentRuntimePageProps {
  params: Promise<{ agentId: string }>;
}

export default function AgentRuntimePage({ params }: AgentRuntimePageProps) {
  const { agentId } = use(params);
  const model = useAgentRuntimePageModel(agentId);

  if (model.isLoading) {
    return <AgentRuntimeLoadingState />;
  }

  return (
    <div className="flex h-full w-full flex-col overflow-hidden bg-background">
      {model.leaveGuardNode}
      <AgentRuntimeWorkbench model={model} />
      <AgentRuntimeDialogs model={model} />
    </div>
  );
}
