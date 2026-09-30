'use client';

import React, { createContext, useContext, useMemo } from 'react';
import type { AgentType } from '@/services/types/agent';

// Context payload for the Workflow Editor
export interface WorkflowEditorContextValue {
  agentId: string;
  /** Stable UUID of the editable Workflow draft used by Connector Target Bindings. */
  workflowId: string;
  agentType: AgentType;
  workspaceId: string;
}

const WorkflowEditorContext = createContext<WorkflowEditorContextValue | null>(null);

interface WorkflowEditorProviderProps {
  value: WorkflowEditorContextValue;
  children: React.ReactNode;
}

// Provider that supplies agentId and agentType to the workflow UI tree
export const WorkflowEditorProvider: React.FC<WorkflowEditorProviderProps> = ({
  value,
  children,
}) => {
  const { agentId, workflowId, agentType, workspaceId } = value;
  // Memoize to avoid needless re-renders for deep trees
  const memoValue = useMemo(
    () => ({ agentId, workflowId, agentType, workspaceId }),
    [agentId, agentType, workflowId, workspaceId]
  );
  return (
    <WorkflowEditorContext.Provider value={memoValue}>{children}</WorkflowEditorContext.Provider>
  );
};

// Hook for consuming the workflow editor context
export const useWorkflowEditor = (): WorkflowEditorContextValue => {
  const ctx = useContext(WorkflowEditorContext);
  if (!ctx) {
    throw new Error('useWorkflowEditor must be used within a WorkflowEditorProvider');
  }
  return ctx;
};
