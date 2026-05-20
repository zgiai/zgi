'use client';

import React, { createContext, useContext, useMemo } from 'react';
import type { AgentType } from '@/services/types/agent';

// Context payload for the Workflow Editor
export interface WorkflowEditorContextValue {
  agentId: string;
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
  // Memoize to avoid needless re-renders for deep trees
  const memoValue = useMemo(() => value, [value.agentId, value.agentType, value.workspaceId]);
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
