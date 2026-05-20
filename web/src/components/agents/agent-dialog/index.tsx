'use client';

import { CreateAgentDialog } from './create-dialog';
import { EditAgentDialog } from './edit-dialog';

interface AgentDialogProps {
  open: boolean;
  mode: 'create' | 'edit';
  onOpenChange: (open: boolean) => void;
  agentId?: string;
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
export function AgentDialog({ open, mode, onOpenChange, agentId }: AgentDialogProps) {
  if (mode === 'create') {
    return <CreateAgentDialog open={open} onOpenChange={onOpenChange} />;
  }
  if (!agentId) {
    return null;
  }
  return <EditAgentDialog open={open} onOpenChange={onOpenChange} agentId={agentId} />;
}

export { CreateAgentDialog, EditAgentDialog };
export default AgentDialog;
