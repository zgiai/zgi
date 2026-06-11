'use client';

import { use } from 'react';
import ApiDocsTab from '@/components/agents/api/api-docs-tab';
import AgentApiAccessGuard from '@/components/agents/api/agent-api-access-guard';

interface AgentApiDocsPageProps {
  params: Promise<{ agentId: string }>;
}

export default function AgentApiDocsPage({ params }: AgentApiDocsPageProps) {
  const { agentId } = use(params);

  return (
    <AgentApiAccessGuard agentId={agentId}>
      {({ agentType }) => <ApiDocsTab agentType={agentType} />}
    </AgentApiAccessGuard>
  );
}
