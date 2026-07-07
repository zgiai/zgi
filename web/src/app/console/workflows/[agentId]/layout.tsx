'use client';

import React from 'react';
import AgentDetailLayout from '@/components/agents/agent-detail-layout';

export default function WorkflowLayout({ children }: { children: React.ReactNode }) {
  return <AgentDetailLayout routeKind="workflow">{children}</AgentDetailLayout>;
}
