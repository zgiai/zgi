import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';

import {
  getCanvasPreviewRows,
  groupWorkflowRunItems,
} from '../src/components/workflow/ui/workflow-run-nodes-list/utils.ts';

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
const workflowRunResults = read(
  'src/components/workflow/ui/workflow-run-panel/components/results.tsx'
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
const webappTransport = read('src/hooks/webapp/use-webapp-transport.ts');
const webappTransportEvents = read('src/hooks/webapp/use-webapp-transport/events.ts');
const approvalRuntimeEvents = read('src/components/workflow/approval/runtime-events.ts');
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
assert.match(workflowEditor, /isSelected \? 2_000 : 1_000/);
assert.match(workflowEditor, /runtimeLogPopoverOpenByNodeId\[node\.id\]/);
assert.match(workflowEditor, /const selectedHistoryNodes = selectedHistorySnapshot\?\.nodes/);
assert.match(workflowEditor, /const selectedHistoryViewport = selectedHistorySnapshot\?\.viewport/);
assert.match(workflowEditor, /historyViewNodesCacheRef/);
assert.match(workflowEditor, /historyViewNodesCacheRef\.current\?\.signature === signature/);
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
  workflowRunResults,
  /<MarkdownViewer[\s\S]*?preserveSoftBreaks/,
  'workflow run results must preserve authored soft line breaks'
);
assert.match(
  workflowConversationHistory,
  /<MarkdownViewer[\s\S]*?preserveSoftBreaks/,
  'workflow history must render the same line breaks as live output'
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

console.log('Workflow canvas runtime result regression checks passed.');
