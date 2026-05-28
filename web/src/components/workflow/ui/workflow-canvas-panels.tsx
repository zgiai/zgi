import React from 'react';
import { AgentType } from '@/services/types/agent';
import { PanelStackProvider } from '../hooks';
import { useActivePanel } from '../hooks/use-active-panel';
import { useWorkflowStore } from '../store';
import NodeFloatingPanel from './node-floating-panel';
import WorkflowRunPanel from './workflow-run-panel';
import WorkflowChatPanel from './workflow-chat-panel';
import { ConversationHistoryPanel } from './conversation-history-panel';
import ConversationVariablesPanel from './conversation-variables-panel';
import EnvironmentVariablesPanel from './environment-variables-panel';
import FeaturesPanel from './features-panel';
import WorkflowBottomToolbar from './workflow-bottom-toolbar';
import WorkflowMinimap from './workflow-minimap';
import NodeLeftPanel from './node-left-panel';
import { isWorkflowDebugPanelActive } from '../hooks/use-debug-focus-mode';

interface WorkflowCanvasPanelsProps {
  agentType: string;
  agentId: string;
  agentName?: string;
  agentIconType?: string;
  agentIcon?: string;
  agentIconUrl?: string;
  isReadOnly: boolean;
  draggingNodeType: string | null;
  temporarilyHidden: boolean;
}

const DeferredPanel: React.FC<{ children: () => React.ReactNode; delay?: number }> = ({
  children,
  delay = 100,
}) => {
  const [shouldRender, setShouldRender] = React.useState(false);

  React.useEffect(() => {
    const timer = window.setTimeout(() => setShouldRender(true), delay);
    return () => window.clearTimeout(timer);
  }, [delay]);

  if (!shouldRender) return null;
  return <>{children()}</>;
};

export function WorkflowCanvasPanels({
  agentType,
  agentId,
  agentName,
  agentIconType,
  agentIcon,
  agentIconUrl,
  isReadOnly,
  draggingNodeType,
  temporarilyHidden,
}: WorkflowCanvasPanelsProps) {
  const activePanel = useActivePanel(s => s.active);
  const setActivePanel = useActivePanel(s => s.setActive);
  const selectedRunId = useWorkflowStore.use.selectedRunId();
  const focusModeActive = isWorkflowDebugPanelActive(activePanel);

  return (
    <PanelStackProvider>
      {!isReadOnly && (
        <DeferredPanel>{() => <NodeLeftPanel focusModeActive={focusModeActive} />}</DeferredPanel>
      )}

      <div className="absolute bottom-0 left-0 z-10">
        <WorkflowMinimap />
      </div>

      <NodeFloatingPanel temporarilyHidden={temporarilyHidden || focusModeActive} />

      {agentType === AgentType.WORKFLOW && (
        <WorkflowRunPanel
          open={activePanel === 'run'}
          temporarilyHidden={temporarilyHidden}
          onClose={() => setActivePanel(null)}
          agentId={agentId}
        />
      )}

      {agentType === AgentType.CONVERSATIONAL_AGENT && (
        <ConversationHistoryPanel
          open={activePanel === 'conversation-history' && Boolean(selectedRunId)}
          temporarilyHidden={temporarilyHidden}
          agentId={agentId}
        />
      )}

      {agentType === AgentType.CONVERSATIONAL_AGENT && (
        <WorkflowChatPanel
          open={activePanel === 'chat'}
          temporarilyHidden={temporarilyHidden}
          onClose={() => setActivePanel(null)}
          agentId={agentId}
          agentName={agentName}
          agentIconType={agentIconType}
          agentIcon={agentIcon}
          agentIconUrl={agentIconUrl}
        />
      )}

      {agentType === AgentType.CONVERSATIONAL_AGENT && (
        <ConversationVariablesPanel
          open={activePanel === 'conversation-variables'}
          temporarilyHidden={temporarilyHidden}
          onClose={() => setActivePanel(null)}
        />
      )}

      <EnvironmentVariablesPanel
        open={activePanel === 'environment-variables'}
        temporarilyHidden={temporarilyHidden}
        onClose={() => setActivePanel(null)}
      />

      {agentType === AgentType.CONVERSATIONAL_AGENT && (
        <FeaturesPanel
          open={activePanel === 'features'}
          temporarilyHidden={temporarilyHidden}
          onClose={() => setActivePanel(null)}
          agentName={agentName}
          agentIconType={agentIconType}
          agentIcon={agentIcon}
          agentIconUrl={agentIconUrl}
        />
      )}

      {!isReadOnly && !draggingNodeType && <WorkflowBottomToolbar />}
    </PanelStackProvider>
  );
}

export default WorkflowCanvasPanels;
