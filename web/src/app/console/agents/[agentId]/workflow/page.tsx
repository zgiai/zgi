import { redirect } from 'next/navigation';

interface LegacyWorkflowPageProps {
  params: Promise<{ agentId: string }>;
}

export default async function LegacyWorkflowPage({ params }: LegacyWorkflowPageProps) {
  const { agentId } = await params;
  redirect(`/console/workflows/${agentId}`);
}
