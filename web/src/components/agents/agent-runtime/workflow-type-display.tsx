'use client';

import { MessageSquareQuote, Workflow } from 'lucide-react';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { AgentType } from '@/services/types/agent';

interface AgentWorkflowTypeDisplayProps {
  agentType?: AgentType | string | null;
  className?: string;
}

function isConversationalWorkflow(agentType?: AgentType | string | null) {
  return String(agentType || '').toUpperCase() === AgentType.CONVERSATIONAL_AGENT;
}

export function AgentWorkflowTypeIcon({ agentType, className }: AgentWorkflowTypeDisplayProps) {
  const Icon = isConversationalWorkflow(agentType) ? MessageSquareQuote : Workflow;
  return <Icon className={cn('size-4', className)} />;
}

export function AgentWorkflowTypeBadge({ agentType, className }: AgentWorkflowTypeDisplayProps) {
  const t = useT('agents.agentRuntime');
  const label = isConversationalWorkflow(agentType)
    ? t('workflow.conversationalWorkflow')
    : t('workflow.taskWorkflow');

  return (
    <span
      className={cn(
        'inline-flex rounded border bg-muted/40 px-1.5 py-0.5 text-[11px] text-muted-foreground',
        className
      )}
    >
      {label}
    </span>
  );
}
