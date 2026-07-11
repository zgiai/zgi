'use client';

import { use } from 'react';
import ApiKeysTab from '@/components/agents/api/api-keys-tab';
import AgentApiAccessGuard from '@/components/agents/api/agent-api-access-guard';

interface AgentApiKeysPageProps {
  params: Promise<{ agentId: string }>;
}

export default function AgentApiKeysPage({ params }: AgentApiKeysPageProps) {
  const { agentId } = use(params);

  return (
    <AgentApiAccessGuard agentId={agentId}>
      {() => <ApiKeysTab agentId={agentId} />}
    </AgentApiAccessGuard>
  );
}
