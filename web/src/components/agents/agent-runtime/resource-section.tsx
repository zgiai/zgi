'use client';

import type { ReactNode } from 'react';
import { ResourceBindingCard, ResourceBindingSection } from '@/components/common/resource-binding';
import { useT } from '@/i18n';
import type { AgentConfigSection } from './types';
import type { AgentBindingHealthItem } from '@/services/types/agent';
import { AgentBindingHealthBadge } from './binding-health';

interface AgentRuntimeResourceSectionProps {
  title: string;
  section: AgentConfigSection;
  open: boolean;
  count: number;
  addLabel: string;
  helpText: string;
  emptyText: string;
  isLoading: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onAdd: () => void;
  addTooltip?: string;
  readOnly?: boolean;
  children: ReactNode;
}

interface AgentRuntimeResourceCardProps {
  icon: ReactNode;
  title: string;
  description?: ReactNode;
  action?: ReactNode;
  children?: ReactNode;
  error?: boolean;
  healthItem?: AgentBindingHealthItem;
}

export function AgentRuntimeResourceSection({
  title,
  section,
  open,
  count,
  addLabel,
  helpText,
  emptyText,
  isLoading,
  onToggleSection,
  onAdd,
  addTooltip,
  readOnly = false,
  children,
}: AgentRuntimeResourceSectionProps) {
  const t = useT('agents.agentRuntime');
  return (
    <ResourceBindingSection
      id={`agent-config-section-${section}`}
      title={title}
      open={open}
      count={count}
      countLabel={t('selectedCount', { count })}
      addLabel={addLabel}
      addTooltip={addTooltip}
      helpText={helpText}
      emptyText={emptyText}
      isLoading={isLoading}
      onToggle={() => onToggleSection(section)}
      onAdd={onAdd}
      readOnly={readOnly}
    >
      {children}
    </ResourceBindingSection>
  );
}

export function AgentRuntimeResourceCard({
  icon,
  title,
  description,
  action,
  children,
  error,
  healthItem,
}: AgentRuntimeResourceCardProps) {
  return (
    <ResourceBindingCard
      icon={icon}
      title={title}
      description={description}
      action={action}
      status={<AgentBindingHealthBadge item={healthItem} />}
      error={error}
    >
      {children}
    </ResourceBindingCard>
  );
}
