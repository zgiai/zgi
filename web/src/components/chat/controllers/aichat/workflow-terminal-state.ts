import type { AIChatAgenticTimelineItem } from './types';
import type { NodeInfo, RunStatus } from '@/components/chat/types';

function terminalizeWorkflowNode(node: NodeInfo, status: 'stopped' | 'failed'): NodeInfo {
  const terminalStatus =
    node.status === 'running' || node.status === 'paused' ? status : node.status;
  const terminalizeRounds = (rounds: NodeInfo['iterationRounds']) =>
    rounds?.map(round => ({
      ...round,
      nodes: (round.nodes ?? []).map(child => terminalizeWorkflowNode(child, status)),
    }));
  return {
    ...node,
    status: terminalStatus,
    iterationRounds: terminalizeRounds(node.iterationRounds),
    loopRounds: terminalizeRounds(node.loopRounds),
  };
}

export function terminalizeWorkflowNodes(nodes: NodeInfo[], status: RunStatus): NodeInfo[] {
  if (status !== 'stopped' && status !== 'error') return nodes;
  const nodeStatus = status === 'stopped' ? 'stopped' : 'failed';
  return nodes.map(node => terminalizeWorkflowNode(node, nodeStatus));
}

export function terminalizeWorkflowTimeline(
  timeline: AIChatAgenticTimelineItem[] | undefined,
  status: 'stopped' | 'error'
): AIChatAgenticTimelineItem[] {
  return (timeline ?? [])
    .filter(
      item =>
        !(
          item.type === 'progress_text' &&
          !item.content.trim() &&
          (item.transient === true || Boolean(item.phase))
        )
    )
    .map(item =>
      item.type === 'workflow_run' &&
      (item.status === 'running' ||
        item.status === 'pending_approval' ||
        item.status === 'pending_question')
        ? { ...item, status, nodes: terminalizeWorkflowNodes(item.nodes, status) }
        : item
    );
}
