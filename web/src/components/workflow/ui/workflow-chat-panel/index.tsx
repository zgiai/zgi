import React from 'react';
import { Panel } from '@xyflow/react';

import Chat from '@/components/chat';

import { getRightPanelMotionClassName, getRightPanelMotionStyle } from '../right-panel-motion';
import { PanelHeader } from './components/panel-header';
import { RunWarningDialog } from './components/run-warning-dialog';
import { useWorkflowChatPanelState } from './hooks/use-workflow-chat-panel-state';
import type { WorkflowChatPanelProps } from './types';

const WorkflowChatPanel: React.FC<WorkflowChatPanelProps> = props => {
  const { open, temporarilyHidden = false } = props;
  const state = useWorkflowChatPanelState(props);

  if (!open) return null;

  return (
    <Panel
      position="top-right"
      aria-hidden={temporarilyHidden}
      className={getRightPanelMotionClassName(
        `relative p-0 bg-primary-foreground border border-muted rounded-lg shadow-lg h-[calc(100%-120px)] overflow-hidden ${state.isResizing ? 'select-none' : ''} ${state.shake ? 'workflow-panel-attention' : ''}`,
        temporarilyHidden
      )}
      style={{
        ...getRightPanelMotionStyle(state.panelStyle, temporarilyHidden),
        ...state.panelWidthStyle,
      }}
    >
      <div
        aria-hidden="true"
        className="absolute left-0 top-0 z-20 h-full w-2 cursor-ew-resize transition-colors hover:bg-primary/10"
        {...state.resizeHandleProps}
      />
      <div
        className="flex flex-col h-full"
        onContextMenu={e => {
          e.stopPropagation();
        }}
      >
        <PanelHeader
          agentId={state.agentId}
          query={state.debugRunsQuery}
          canViewRuntimeLogs={state.canViewRuntimeLogs}
          onSelectDebugRun={state.handleSelectDebugRun}
          onReset={state.handleReset}
          onClose={state.onClose}
        />
        <div className="flex-1 min-h-0 flex flex-col overflow-hidden">
          <div className="flex-1 min-h-0">
            <Chat
              key={`${state.varsSig}-${state.convId}`}
              className="h-full"
              mode="singleTest"
              conversation={{
                id: state.convId,
                conversationId: state.chatConv?.conversationId ?? '',
              }}
              onSend={state.handleSend}
              onStop={state.handleStop}
              features={state.features}
              enableUpload={state.features?.file_upload?.enabled ?? true}
              openingGuide={state.openingGuide}
              openingGuideBrand={state.openingGuideBrand}
              suggestions={state.suggestedQuestions}
              inputDisabled={undefined}
              showWorkflowRunHeader
              showWorkflowDetail
              showWorkflowNodeDetail
              placeholder={state.placeholder}
              toolbarForm={state.toolbarFormSpec}
              inputTopNotice={state.inputTopNotice}
              inputReplacement={state.approvalInputReplacement}
              sendDisabled={state.sendDisabled}
              isRunning={state.isRunning}
              isStopping={state.isStopping}
            />
            <RunWarningDialog
              open={state.runWarnOpen}
              dontWarnAgain={state.dontWarnAgain}
              onOpenChange={state.setRunWarnOpen}
              onDontWarnAgainChange={state.setDontWarnAgain}
              onViewErrors={state.handleViewRunErrors}
              onContinue={state.handleContinueRunWarning}
            />
          </div>
        </div>
      </div>
    </Panel>
  );
};

export default WorkflowChatPanel;
