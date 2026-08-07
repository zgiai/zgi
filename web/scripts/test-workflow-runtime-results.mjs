import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';

import {
  getCanvasPreviewRows,
  groupWorkflowRunItems,
} from '../src/components/workflow/ui/workflow-run-nodes-list/utils.ts';
import {
  extractWorkflowRunContainerContext,
  getWorkflowRunExecutionId,
  isVisibleWorkflowRunExecutionStatus,
} from '../src/utils/workflow/run-events.ts';
import {
  createWorkflowSnapshotPauseEvent,
  parseApprovalPausedEvent,
} from '../src/components/workflow/approval/runtime-events.ts';
import { parseQuestionAnswerPausedEvent } from '../src/components/workflow/question-answer/runtime-events.ts';
import {
  pinWorkflowRunId,
  resolveWorkflowRunId,
} from '../src/utils/workflow/run-identity.js';

const root = process.cwd();
const read = relativePath => readFileSync(path.join(root, relativePath), 'utf8');

const grouped = groupWorkflowRunItems([
  {
    title: 'Child node',
    nodeId: 'child-1',
    executionId: 'round-1',
    nodeType: 'code',
    status: 'succeeded',
    nodeOutput: { round: 1 },
  },
  {
    title: 'Child node',
    nodeId: 'child-1',
    executionId: 'round-2',
    nodeType: 'code',
    status: 'succeeded',
    nodeOutput: { round: 2 },
  },
]);

assert.equal(grouped.length, 1);
assert.equal(grouped[0].executions.length, 2);
assert.equal(grouped[0].executions[0].executionId, 'round-1');
assert.equal(grouped[0].executions[1].executionId, 'round-2');

const runtimeDetails = read('src/components/workflow/nodes/custom/node-runtime-log-details.tsx');
const runtimeList = read('src/components/workflow/ui/workflow-run-nodes-list/index.tsx');
const runtimeStructuredView = read(
  'src/components/workflow/ui/workflow-run-nodes-list/runtime-structured-view.tsx'
);
const workflowEditor = read('src/components/workflow/index.tsx');
const graphHelpers = read('src/components/workflow/store/helpers/graph.ts');
const modeSelection = read('src/components/workflow/store/slices/mode-selection.ts');
const resizeHandle = read('src/components/workflow/nodes/custom/node-resize-handle.tsx');
const runsDropdown = read('src/components/workflow/ui/workflow-runs-dropdown/index.tsx');
const consoleSidebar = read('src/components/console/console-sidebar.tsx');
const resourceSidebar = read('src/components/common/resource-sidebar.tsx');
const contextualAIChatContext = read(
  'src/components/aichat/contextual/contextual-ai-chat-context.tsx'
);
const workflowService = read('src/services/workflow.service.ts');
const chatApi = read('src/components/chat/hooks/use-chat-api.ts');
const workflowRunEventsStream = read('src/hooks/workflow/use-workflow-run-events-stream.ts');
const workflowChatPanelState = read(
  'src/components/workflow/ui/workflow-chat-panel/hooks/use-workflow-chat-panel-state.tsx'
);
const workflowRunPanel = read('src/components/workflow/ui/workflow-run-panel/index.tsx');
const workflowRunHistoryContent = read(
  'src/components/workflow/ui/workflow-run-panel/components/history-content.tsx'
);
const workflowNodePanel = read('src/components/workflow/ui/node-floating-panel.tsx');
const workflowCanvasPanels = read('src/components/workflow/ui/workflow-canvas-panels.tsx');
const workflowCanvasWithDnd = read('src/components/workflow/canvas-with-dnd.tsx');
const workflowRunResults = read(
  'src/components/workflow/ui/workflow-run-panel/components/results.tsx'
);
const workflowRunHistory = read(
  'src/components/workflow/ui/workflow-run-panel/utils/history-view-data.ts'
);
const workflowConversationHistory = read(
  'src/components/workflow/ui/conversation-history-panel/index.tsx'
);
const chatMessageItem = read('src/components/chat/ui/message-item/index.tsx');
const markdownViewer = read('src/components/common/markdown-viewer.tsx');
const workflowDraftRunStream = read('src/hooks/workflow/use-run-workflow-draft-stream.ts');
const workflowChatDraftRunStream = read('src/hooks/workflow/use-run-workflow-chat-draft-stream.ts');
const webappService = read('src/services/webapp.service.ts');
const webappRunStream = read('src/hooks/webapp/use-run-webapp-workflow-stream.ts');
const webappRun = read('src/components/webapp/run/index.tsx');
const workflowRunNodeAccumulator = read('src/utils/webapp/workflow-run-node-accumulator.ts');
const webappTransport = read('src/hooks/webapp/use-webapp-transport.ts');
const webappTransportEvents = read('src/hooks/webapp/use-webapp-transport/events.ts');
const approvalRuntimeEvents = read('src/components/workflow/approval/runtime-events.ts');
const approvalRuntimeHook = read(
  'src/components/workflow/approval/use-approval-runtime-events.ts'
);
const workflowPauseEvents = read('src/components/workflow/runtime/pause-events.ts');
const aiChatWorkflowReducer = read(
  'src/components/chat/controllers/aichat/reducers/workflow.ts'
);
const sseClient = read('src/lib/http/sse-client.ts');
const chatWithController = read('src/components/chat/chat-with-controller.tsx');
const singleChatController = read('src/components/chat/controllers/single-chat-controller.ts');

assert.doesNotMatch(runtimeDetails, /max-h-\[min\(420px,calc\(100vh-160px\)\)\]/);
assert.doesNotMatch(runtimeDetails, /TooltipTrigger|TooltipProvider/);
assert.doesNotMatch(runsDropdown, /TooltipTrigger/);
assert.doesNotMatch(runsDropdown, /DropdownMenuTrigger|DropdownMenuContent/);
assert.match(runsDropdown, /role="menu"/);
assert.doesNotMatch(consoleSidebar, /function CollapsedNavTooltip[\s\S]*?TooltipTrigger/);
assert.doesNotMatch(resourceSidebar, /function ResourceSidebarTooltip[\s\S]*?TooltipTrigger/);
assert.match(contextualAIChatContext, /const itemsSignature = useMemo/);
assert.match(contextualAIChatContext, /\[itemsSignature, priority, registerItems/);
assert.match(modeSelection, /sameGraphPart/);
assert.match(modeSelection, /cloned\.nodes = current\.nodes/);
assert.doesNotMatch(runtimeDetails, /setRuntimeResultFootprint|ResizeObserver/);
assert.match(runtimeDetails, /absolute left-0 top-\[calc\(100%\+6px\)\]/);
assert.match(runtimeStructuredView, /max-h-40 overflow-y-auto/);
assert.match(runtimeStructuredView, /max-h-\[200px\] overflow-y-auto/);
assert.match(runtimeList, /isCanvasDetailOnly && group\.executions\.length > 1/);
assert.match(runtimeList, /setSelectedExecutionByNode/);
assert.match(runtimeList, /selectedRoundByNode/);
assert.match(runtimeList, /visibleRounds\.map/);
assert.match(runtimeList, /bounded=\{isCanvasVariant\}/);
assert.match(runtimeList, /row\.labelKind === 'variable'/);
assert.match(runtimeList, /text-\[11px\] font-semibold leading-4 tracking-tight/);
assert.match(runtimeStructuredView, /bounded \? 'text-primary' : 'text-muted-foreground\/70'/);
assert.match(workflowEditor, /isSelected \? 2_000 : 1_000/);
assert.match(workflowEditor, /runtimeLogPopoverOpenByNodeId\[node\.id\]/);
assert.match(workflowEditor, /const selectedHistoryNodes = selectedHistorySnapshot\?\.nodes/);
assert.match(workflowEditor, /const selectedHistoryViewport = selectedHistorySnapshot\?\.viewport/);
assert.match(workflowEditor, /historyViewNodesCacheRef/);
assert.match(workflowEditor, /historyViewNodesCacheRef\.current\?\.signature === signature/);
assert.match(workflowEditor, /selectionChanged = isHistoryMode/);
assert.match(workflowEditor, /selectionChanged \? \{ selected: isSelected \}/);
assert.match(workflowEditor, /deriveContainerLayoutSizes/);
assert.match(graphHelpers, /calculateContainerMinimumSize/);
assert.match(graphHelpers, /deriveContainerLayoutSizes/);
assert.match(graphHelpers, /CONTAINER_RUNTIME_FOOTER_HEIGHT = 40/);
assert.match(graphHelpers, /CONTAINER_BOTTOM_SAFE_INSET/);
assert.match(graphHelpers, /parentH - childH - CONTAINER_BOTTOM_SAFE_INSET/);
assert.match(graphHelpers, /height: maxBottom \+ CONTAINER_BOTTOM_SAFE_INSET/);
assert.doesNotMatch(graphHelpers, /runtimeResultFootprintByNodeId|RUNTIME_RESULT_GAP/);
assert.match(graphHelpers, /getDepth\(right\) - getDepth\(left\)/);
assert.match(resizeHandle, /setActiveResizeNodeId\(nodeId\)/);
assert.match(resizeHandle, /calculateContainerMinimumSize/);
assert.match(resizeHandle, /deriveContainerLayoutSizes/);

const eventApplyIndex = workflowService.indexOf('handlers[event]?.(payload);');
const cursorAdvanceIndex = workflowService.indexOf(
  'callbacks.onEventCursor?.(sequence);',
  eventApplyIndex
);
assert.notEqual(eventApplyIndex, -1, 'workflow event dispatcher must apply the event');
assert.notEqual(cursorAdvanceIndex, -1, 'workflow event dispatcher must advance the cursor');
assert.ok(
  eventApplyIndex < cursorAdvanceIndex,
  'workflow event dispatcher must apply an event before advancing its durable cursor'
);
assert.match(
  chatApi,
  /typeof m\['answer_delta'\] === 'string'/,
  'chat projection must consume V2 workflow answer checkpoints'
);
assert.match(
  chatApi,
  /projection_revision: checkpointRevision/,
  'chat projection must remember checkpoint revisions for replay deduplication'
);
assert.doesNotMatch(
  chatApi,
  /shouldPreserveLocalAnswerForSnapshot/,
  'authoritative replace checkpoints must repair stale or duplicated live answers'
);
assert.match(
  chatApi,
  /answer:\s*checkpointDelta,\s*answerMode:\s*'replace'/,
  'replace checkpoints must replace the local answer projection'
);
assert.doesNotMatch(
  workflowRunEventsStream,
  /revision\s*!==\s*current\s*\+\s*1/,
  'answer revisions may skip values consumed by pause and status projections'
);
assert.match(
  workflowChatPanelState,
  /answer_revision:[\s\S]*message\.projection_revision/,
  'workflow chat snapshots must restore the persisted answer revision'
);
assert.match(
  workflowService,
  /workflow_resumed: callbacks\.onWorkflowResumed/,
  'workflow recovery must dispatch the durable resumed event'
);
assert.match(
  sseClient,
  /workflow_resumed: callbacks\.onWorkflowResumed/,
  'generic SSE transports must dispatch the durable resumed event'
);
assert.match(
  workflowRunPanel,
  /case 'workflow_resumed':[\s\S]*handleWorkflowResumed\(event\)/,
  'the workflow debugger must leave the paused projection after resume'
);
assert.match(
  workflowDraftRunStream,
  /streamGenerationRef/,
  'draft reruns must isolate callbacks from superseded streams'
);
assert.match(
  workflowChatDraftRunStream,
  /gracefulStreamBoundaryRef\.current\s*=\s*true/,
  'a paused workflow chat POST stream must be treated as a graceful transport boundary'
);
assert.match(
  workflowChatDraftRunStream,
  /suppressTransportErrorNotification:\s*true/,
  'workflow chat transport recovery must suppress false global network errors'
);
assert.match(
  workflowChatDraftRunStream,
  /onTransportError:[\s\S]*?transportRecoveryRequestedRef\.current\s*=\s*true;[\s\S]*?recover\(runId, error\)/,
  'an interrupted workflow chat stream must hand off to durable event recovery'
);
assert.match(
  workflowDraftRunStream,
  /onTransportInterrupted/,
  'an interrupted draft stream must hand off to durable event recovery'
);
assert.match(
  workflowDraftRunStream,
  /transportRecoveryRequested\s*=\s*true;[\s\S]*?isRunningRef\.current\s*=\s*false;[\s\S]*?setIsRunning\(false\);[\s\S]*?recover\(workflowRunId, error\)/,
  'durable recovery must release the superseded POST stream running state'
);
assert.match(
  workflowDraftRunStream,
  /suppressTransportErrorNotification:\s*true/,
  'recoverable draft transport interruptions must not show a false network error'
);
assert.match(
  webappService,
  /onTransportError:\s*opts\?\.onTransportError[\s\S]*suppressTransportErrorNotification:\s*opts\?\.suppressTransportErrorNotification/,
  'published workflow transport options must reach the shared SSE client'
);
assert.match(
  workflowService,
  /opts\?\.transport === 'webapp' \? webappHttp : this\.client/,
  'anonymous WebApp durable recovery must use the WebApp-aware SSE transport'
);
assert.doesNotMatch(
  workflowService.slice(
    workflowService.indexOf('sseWorkflowRunEvents('),
    workflowService.indexOf('async getWorkflowRunDetail(')
  ),
  /wrappedCallbacks\.onError/,
  'durable stream transport failures must not be reported as workflow business failures'
);
assert.match(
  `${webappTransport}\n${webappRun}`,
  /useWorkflowRunEventsStream\(\{ transport: 'webapp' \}\)/,
  'published WebApp durable event consumers must select WebApp authentication'
);
assert.match(
  webappRunStream,
  /onWorkflowPaused:\s*p\s*=>\s*\{[\s\S]*gracefulStreamBoundary\s*=\s*true/,
  'a published workflow pause must be treated as a graceful POST stream boundary'
);
assert.match(
  webappRunStream,
  /suppressTransportErrorNotification:\s*true[\s\S]*onTransportError:[\s\S]*requestTransportRecovery\(error\)/,
  'published workflow transport handoff must suppress false network errors and recover durable events'
);
const transportErrorHandler = sseClient.slice(
  sseClient.indexOf('private handleStreamTransportError'),
  sseClient.indexOf('private dispatchSseEvent')
);
assert.match(
  transportErrorHandler,
  /!options\.skipErrorHandling\s*&&\s*!options\.suppressTransportErrorNotification/,
  'the global network toast must be suppressible only for an established recoverable stream'
);
assert.match(
  workflowRunPanel,
  /onTransportInterrupted:\s*canViewRuntimeEvents[\s\S]*startApprovalResumeEventStream/,
  'the workflow debugger must recover the current run from snapshot and durable events'
);
assert.match(
  workflowRunPanel,
  /!isTerminalRunSummary\s*&&\s*\(isRunning\s*\|\|\s*runSummaryStatus\s*===\s*'running'\)/,
  'a durable terminal summary must override stale transport running state'
);
const workflowDraftContent = read(
  'src/components/workflow/ui/workflow-run-panel/components/draft-content.tsx'
);
assert.match(
  workflowDraftContent,
  /isWaitingForOutput = isRunning && !hasVisibleResult && !hasPendingInteraction/,
  'a running task without output must render a waiting state instead of an empty JSON result'
);
assert.match(
  workflowDraftContent,
  /shouldHideEmptyResult = hasPendingInteraction && !hasVisibleResult/,
  'approval and question cards must not be followed by a redundant empty result section'
);
assert.match(
  workflowChatPanelState,
  /case 'workflow_resumed':[\s\S]*setIsConversationPaused\(false\)/,
  'workflow chat must leave the paused projection after resume'
);
assert.match(
  workflowChatPanelState,
  /onTransportInterrupted:[\s\S]*startApprovalResumeEventStreamRef\.current/,
  'workflow chat transport interruption must observe the established durable run'
);
assert.match(
  workflowChatPanelState,
  /onTransportInterrupted:[\s\S]*setIsDurableRunActive\(true\)/,
  'workflow chat recovery must keep the logical run active after the POST transport closes'
);
assert.match(
  workflowChatPanelState,
  /if \(isRunning \|\| isDurableRunActive\) return;/,
  'workflow chat must reject a new message while either transport or durable recovery is running'
);
assert.match(
  workflowChatPanelState,
  /isRunning:\s*isRunning \|\| isDurableRunActive/,
  'workflow chat composer must remain in running mode throughout durable recovery'
);
assert.match(
  webappRun,
  /case 'workflow_resumed':[\s\S]*setApprovalPaused\(false\)/,
  'published workflow runs must leave the paused projection after resume'
);
assert.match(
  webappRun,
  /onIterationStarted:\s*streamPayload\s*=>[\s\S]*durableRunNodeAccumulator\.onIterationStarted\(streamPayload\)/,
  'published task WebApps must project durable iteration events through the shared accumulator'
);
assert.match(
  webappRun,
  /onLoopStarted:\s*streamPayload\s*=>[\s\S]*durableRunNodeAccumulator\.onLoopStarted\(streamPayload\)/,
  'published task WebApps must project durable loop events through the shared accumulator'
);
assert.match(
  webappRun,
  /durableRunNodeAccumulator\.replaceSnapshot\([\s\S]*executionItems\.map\(nodeInfoFromWorkflowRunItem\)/,
  'published task WebApps must seed container projection from the authoritative snapshot'
);
assert.match(
  webappRun,
  /await start\(runPayload, undefined, \{[\s\S]*onTransportInterrupted: recoverInterruptedWorkflowRun/,
  'published task WebApps must hand an interrupted POST stream over to durable run events'
);
assert.match(
  webappRun,
  /question_answer_option_id: choice\.id,[\s\S]*onTransportInterrupted: recoverInterruptedWorkflowRun/,
  'question continuations must use the same durable recovery handoff'
);
assert.match(
  webappRunStream,
  /onNodeStarted: p => \{[\s\S]*captureWorkflowRunId\(p\)/,
  'the initial stream must recover the workflow run id even when workflow_started is not delivered'
);
assert.match(
  workflowRunNodeAccumulator,
  /const replaceSnapshot = \(nodes: NodeInfo\[\]\) =>/,
  'the shared workflow accumulator must support snapshot hydration before replaying event tails'
);
assert.match(
  workflowRunNodeAccumulator,
  /if \(handleContainerLifecycle\(payload, node, finished\)\) return;/,
  'generic container node completion must not replace accumulated iteration or loop rounds'
);
assert.match(
  webappRun,
  /iterationRounds: next\.iterationRounds \?\? item\.iterationRounds/,
  'partial lifecycle projections must preserve accumulated iteration rounds'
);
assert.match(
  webappRun,
  /loopRounds: next\.loopRounds \?\? item\.loopRounds/,
  'partial lifecycle projections must preserve accumulated loop rounds'
);
assert.match(
  approvalRuntimeEvents,
  /activePause\.reasons\)[\s\S]*filter\(isPendingPauseReason\)/,
  'workflow snapshots must only restore pending interaction reasons'
);
assert.match(
  approvalRuntimeEvents,
  /if \(!isActionablePause\(pause\)\) return null/,
  'submitted, resuming, and closed pauses must not rebuild actionable approval UI'
);
assert.doesNotMatch(
  `${webappTransport}\n${webappTransportEvents}\n${webappRun}\n${workflowChatPanelState}`,
  /hasUnresolvedApprovals|shouldKeepApprovalPending/,
  'durable terminal events must never be downgraded by stale in-memory approval state'
);
assert.doesNotMatch(
  webappTransportEvents,
  /approvalCursor/,
  'workflow recovery must use the per-run cursor instead of a global approval cursor'
);
assert.match(
  chatWithController,
  /const currentRuntimeStatus =\s*latestRunStatus \|\|\s*latestMessage\?\.clientState\?\.status/,
  'the latest message terminal status must override stale conversation runtime metadata'
);
assert.doesNotMatch(
  chatWithController,
  /persistedRuntimeStatus === 'running'/,
  'stale conversation metadata must not independently keep the stop button visible'
);
assert.match(
  singleChatController,
  /runtime_status: 'idle',[\s\S]*active_workflow_run_id: undefined/,
  'terminal callbacks must release the active run in the conversation summary projection'
);
assert.match(
  markdownViewer,
  /preserveSoftBreaks\s*&&\s*'whitespace-pre-wrap'/,
  'MarkdownViewer must be able to render workflow-authored soft line breaks'
);
assert.match(
  chatMessageItem,
  /<MarkdownViewer[\s\S]*?preserveSoftBreaks/,
  'chat answers must preserve workflow round separators'
);
assert.match(
  chatMessageItem,
  /workflowPauseStatus[\s\S]*?workflowRunMonitor\.pendingApprovalTitle[\s\S]*?workflowRunMonitor\.pendingQuestionTitle/,
  'empty paused workflow messages must render an explicit user-facing pause state'
);
assert.match(
  chatMessageItem,
  /isEmptyStoppedWorkflow[\s\S]*?workflowRunMonitor\.stoppedTitle[\s\S]*?workflowRunMonitor\.stoppedDescription/,
  'an empty stopped workflow message must render a clear stopped state instead of a placeholder'
);
assert.match(
  chatMessageItem,
  /role="status"[\s\S]*?aria-live="polite"/,
  'the workflow pause state must be exposed to assistive technology'
);
assert.match(
  workflowRunResults,
  /<MarkdownViewer[\s\S]*?preserveSoftBreaks/,
  'workflow run results must preserve authored soft line breaks'
);
assert.match(
  workflowConversationHistory,
  /<MarkdownViewer[\s\S]*?preserveSoftBreaks/,
  'workflow history must render the same line breaks as live output'
);
assert.match(workflowConversationHistory, /id: 'conversation-history',[\s\S]*?order: 1/);
assert.doesNotMatch(workflowConversationHistory, /<RunStatusBadge status=\{inspectorSummary\.status\}/);
assert.doesNotMatch(workflowConversationHistory, /workflow\.workflowRunId/);
assert.match(workflowRunPanel, /!isHistory && canViewRuntimeLogs/);
assert.match(workflowRunHistoryContent, /agents\.workflow\.runOverview/);
assert.match(workflowRunHistoryContent, /agents\.workflow\.nodeDetails/);
assert.match(workflowRunHistoryContent, /agents\.workflow\.finalResult/);
assert.match(workflowNodePanel, /const displayedNodeId = selectedNodeId \?\? activeNodeId/);
assert.doesNotMatch(workflowNodePanel, /historyDeselectTimerRef|shouldReturnToTaskHistory/);
assert.match(workflowEditor, /!isTaskWorkflowHistoryMode[\s\S]*?setActivePanel\(null\)/);
assert.match(modeSelection, /selectedRunId: prepared\.selectedRunId,[\s\S]*?selectedNodeId: null/);
assert.match(workflowCanvasPanels, /const taskRunPanelOpen = isTaskHistory \|\| activePanel === 'run'/);
assert.match(workflowCanvasPanels, /open=\{taskRunPanelOpen\}/);
assert.match(workflowCanvasPanels, /focusModeActive && !isTaskHistory/);
assert.doesNotMatch(workflowCanvasPanels, /taskHistoryNodeInspectorOpen/);
assert.match(
  workflowCanvasWithDnd,
  /const hideRightPanels =\s*!isReadOnly && \(isCanvasInteracting \|\| Boolean\(draggingNodeType\) \|\| createNodePickerOpen\)/
);
assert.match(workflowCanvasWithDnd, /onMoveStart=\{\(\) => \{\s*if \(isReadOnly\) return;/);
assert.match(workflowCanvasWithDnd, /onMoveEnd=\{\(\) => \{\s*if \(isReadOnly\) return;/);

assert.deepEqual(
  extractWorkflowRunContainerContext({
    container_id: 'iteration-1',
    container_type: 'iteration',
    round_index: 2,
  }),
  {
    containerId: 'iteration-1',
    containerType: 'iteration',
    roundIndex: 2,
    iterationId: 'iteration-1',
    iterationIndex: 2,
    loopId: undefined,
    loopIndex: undefined,
  }
);
assert.equal(
  getWorkflowRunExecutionId({ node_execution_id: 'node-execution-v2', execution_id: 'run-owner' }),
  'node-execution-v2',
  'V2 node execution identity must win over the run owner execution id'
);

assert.equal(isVisibleWorkflowRunExecutionStatus('pending'), false);
assert.equal(isVisibleWorkflowRunExecutionStatus('skipped'), false);
assert.equal(isVisibleWorkflowRunExecutionStatus('succeeded'), true);
assert.match(
  workflowRunHistory,
  /\.filter\(rec => isVisibleWorkflowRunExecutionStatus\(rec\.status\)\)/,
  'runtime history must not render untouched graph snapshots as failed executions'
);

const runtimeLabel = (key, params) =>
  params && 'count' in params ? `${key}:${String(params.count)}` : key;
const endRows = getCanvasPreviewRows(
  {
    title: 'End',
    nodeId: 'end-1',
    nodeType: 'end',
    status: 'succeeded',
    nodeOutput: { output: [] },
  },
  runtimeLabel
);
assert.equal(endRows.length, 1);
assert.equal(endRows[0].label, 'output');
assert.deepEqual(endRows[0].value, []);
assert.equal(endRows[0].labelKind, 'variable');

const mixedPausePayload = {
  event: 'workflow_paused',
  data: {
    workflow_run_id: 'run-mixed-pause',
    paused_nodes: ['approval-1', 'question-1'],
    reasons: [
      {
        type: 'approval_required',
        node_id: 'approval-1',
        form_id: 'approval-form-1',
        status: 'pending',
      },
      {
        type: 'question_answer_required',
        node_id: 'question-1',
        question: 'Which option?',
        status: 'pending',
      },
      {
        type: 'question_answer_required',
        node_id: 'question-completed',
        question: 'Already answered',
        status: 'completed',
      },
    ],
  },
};
const mixedApproval = parseApprovalPausedEvent(mixedPausePayload);
const mixedQuestion = parseQuestionAnswerPausedEvent(mixedPausePayload);
assert.equal(mixedApproval.isApproval, true);
assert.equal(mixedQuestion.isQuestionAnswer, true);
assert.deepEqual(mixedApproval.nodeIds, ['approval-1']);
assert.deepEqual(mixedQuestion.nodeIds, ['question-1']);

const approvalSnapshot = {
  event: 'workflow_snapshot',
  data: {
    last_sequence: 5,
    active_pause: {
      pause: {
        id: 'pause-snapshot-1',
        workflow_run_id: 'run-snapshot-1',
        status: 'pending',
      },
      reasons: [
        {
          type: 'approval_required',
          node_id: 'approval-snapshot-1',
          form_id: 'approval-form-snapshot-1',
          status: 'pending',
        },
      ],
    },
  },
};
const reconstructedApprovalPause = createWorkflowSnapshotPauseEvent(approvalSnapshot);
assert.ok(reconstructedApprovalPause);
assert.equal(reconstructedApprovalPause.data.id, 'pause-snapshot-1');
assert.equal(reconstructedApprovalPause.data.workflow_run_id, 'run-snapshot-1');
const reconstructedQuestion = parseQuestionAnswerPausedEvent(reconstructedApprovalPause);
assert.equal(reconstructedQuestion.isQuestionAnswer, false);
assert.equal(
  reconstructedQuestion.workflowRunId,
  'run-snapshot-1',
  'a pause record id must never replace its explicit workflow_run_id'
);
assert.equal(
  resolveWorkflowRunId(reconstructedApprovalPause, {
    fallback: 'run-already-pinned',
    allowLegacyId: true,
  }),
  'run-snapshot-1'
);
assert.equal(
  pinWorkflowRunId(
    'run-already-pinned',
    { data: { id: 'node-execution-1', correlation_id: 'run-other' } },
    { allowLegacyId: true }
  ),
  'run-already-pinned',
  'node and pause events must not overwrite a pinned workflow run id'
);
assert.match(
  workflowPauseEvents,
  /hasQuestionAnswer\s*\?\s*'pending_question'/,
  'a mixed pause must keep the question as its scalar status without dropping approval state'
);
assert.match(
  aiChatWorkflowReducer,
  /parseWorkflowPausedEvent\(payload\)[\s\S]*paused\.preferredStatus\s*\?\?/,
  'Agent embedded workflow pauses must infer question state from durable pause reasons'
);
assert.match(
  sseClient,
  /if \(options\.isTerminalMessage\) \{[\s\S]*return options\.isTerminalMessage/,
  'a transport-specific terminal predicate must override generic workflow pause classification'
);
assert.match(
  approvalRuntimeHook,
  /case 'workflow_resumed':\s*case 'workflow_finished':/,
  'workflow resume must clear stale approval UI before execution continues'
);

console.log('Workflow canvas runtime result regression checks passed.');
