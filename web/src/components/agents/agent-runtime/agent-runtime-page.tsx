'use client';

import {
  AgentRuntimeDialogs,
  AgentRuntimeLoadingState,
  AgentRuntimeWorkbench,
  useAgentRuntimePageModel,
} from '@/components/agents/agent-runtime';
import { PermissionDeniedState } from '@/components/common/permission-gate-state';

interface AgentRuntimePageContentProps {
  agentId: string;
}

export function AgentRuntimePageContent({ agentId }: AgentRuntimePageContentProps) {
  const model = useAgentRuntimePageModel(agentId);

  if (model.isLoading) {
    return <AgentRuntimeLoadingState />;
  }

  if (!model.canOpenAgentRuntimeEditor) {
    return <PermissionDeniedState />;
  }

  return (
    <div className="flex h-full w-full flex-col overflow-hidden bg-background">
      {model.leaveGuardNode}
      <AgentRuntimeWorkbench model={model} />
      <AgentRuntimeDialogs model={model} />
    </div>
  );
}

export default AgentRuntimePageContent;
