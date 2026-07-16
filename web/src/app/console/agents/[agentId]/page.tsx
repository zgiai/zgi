'use client';

import { use } from 'react';
import AgentRuntimePageContent from '@/components/agents/agent-runtime/agent-runtime-page';

interface AgentRuntimePageProps {
  params: Promise<{ agentId: string }>;
}

export default function AgentRuntimePage({ params }: AgentRuntimePageProps) {
  const { agentId } = use(params);
  return <AgentRuntimePageContent agentId={agentId} />;
}
