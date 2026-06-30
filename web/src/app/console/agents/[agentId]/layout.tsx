'use client';

import React from 'react';
import AgentDetailLayout from '@/components/agents/agent-detail-layout';

export default function AgentLayout({ children }: { children: React.ReactNode }) {
  return <AgentDetailLayout routeKind="agent">{children}</AgentDetailLayout>;
}
