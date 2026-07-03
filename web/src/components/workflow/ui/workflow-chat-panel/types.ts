export interface WorkflowChatPanelProps {
  open: boolean;
  temporarilyHidden?: boolean;
  onClose: () => void;
  agentId: string;
  agentName?: string;
  agentIconType?: string;
  agentIcon?: string;
  agentIconUrl?: string;
}
