'use client';

import type { AgentType } from '@/services/types/agent';
import { CreateAgentDialog } from './create-dialog';
import { EditAgentDialog } from './edit-dialog';

interface AgentDialogProps {
  open: boolean;
  mode: 'create' | 'edit';
  onOpenChange: (open: boolean) => void;
  agentId?: string;
  allowedAgentTypes?: AgentType[];
  defaultAgentType?: AgentType;
  hideTypeSelector?: boolean;
}

/**
 * @component AgentDialog
 * @category Feature
 * @status Stable
 * @description Dispatches to create or edit agent dialogs.
 * @usage Use from agent list/card actions to create or edit agent basics.
 * @example
 * <AgentDialog open={open} mode="create" onOpenChange={setOpen} />
 */
export function AgentDialog({
  open,
  mode,
  onOpenChange,
  agentId,
  allowedAgentTypes,
  defaultAgentType,
  hideTypeSelector,
}: AgentDialogProps) {
  if (mode === 'create') {
    return (
      <CreateAgentDialog
        open={open}
        onOpenChange={onOpenChange}
        allowedAgentTypes={allowedAgentTypes}
        defaultAgentType={defaultAgentType}
        hideTypeSelector={hideTypeSelector}
      />
    );
  }
  if (!agentId) {
    return null;
  }
  return <EditAgentDialog open={open} onOpenChange={onOpenChange} agentId={agentId} />;
}

export { CreateAgentDialog, EditAgentDialog };
export default AgentDialog;
