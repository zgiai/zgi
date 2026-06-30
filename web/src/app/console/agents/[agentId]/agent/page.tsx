import { redirect } from 'next/navigation';

interface LegacyAgentRuntimePageProps {
  params: Promise<{ agentId: string }>;
}

export default async function LegacyAgentRuntimePage({ params }: LegacyAgentRuntimePageProps) {
  const { agentId } = await params;
  redirect(`/console/agents/${agentId}`);
}
