'use client';

import { use } from 'react';
import WorkflowDetailPageContent from '@/components/workflow/workflow-detail-page';

interface WorkflowPageProps {
  params: Promise<{ agentId: string }>;
}

export default function WorkflowPage({ params }: WorkflowPageProps) {
  const { agentId } = use(params);
  return <WorkflowDetailPageContent agentId={agentId} />;
}
