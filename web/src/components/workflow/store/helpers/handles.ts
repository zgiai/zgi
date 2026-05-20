// Handle helpers: compute valid source/target handles for each node type
// Used to clean up orphan edges when node data changes

import type { WorkflowNodeData } from '../type';
import type { IfElseNodeData } from '../../nodes/if-else/types';
import { APPROVAL_TIMEOUT_HANDLE, type ApprovalNodeData } from '../../nodes/approval/config';
import {
  QUESTION_ANSWER_DYNAMIC_HANDLE,
  type QuestionAnswerNodeData,
} from '../../nodes/question-answer/config';

/**
 * Compute the set of valid source handle IDs for a node based on its data.
 * Returns undefined if the node type doesn't have dynamic handles (use default behavior).
 */
export function getValidSourceHandles(data: WorkflowNodeData): Set<string> | undefined {
  switch (data.type) {
    case 'if-else': {
      const ifElseData = data as unknown as IfElseNodeData;
      const handles = new Set<string>();
      // Each case has a case_id used as sourceHandle
      (ifElseData.cases || []).forEach(c => {
        if (c.case_id) handles.add(c.case_id);
      });
      // ELSE branch always uses 'false' as handle id
      handles.add('false');
      return handles;
    }
    case 'approval': {
      const approvalData = data as unknown as ApprovalNodeData;
      const handles = new Set<string>();
      (approvalData.approval?.actions || []).forEach(action => {
        if (action.id) handles.add(action.id);
      });
      handles.add(APPROVAL_TIMEOUT_HANDLE);
      return handles;
    }
    case 'question-answer': {
      const questionData = data as unknown as QuestionAnswerNodeData;
      const handles = new Set<string>();
      if (questionData.answer_type !== 'choice') {
        handles.add('source');
        return handles;
      }
      if (questionData.choice_mode === 'dynamic' || questionData.dynamic_choices?.selector?.length) {
        handles.add(QUESTION_ANSWER_DYNAMIC_HANDLE);
        return handles;
      }
      (questionData.choices || []).forEach(choice => {
        if (choice.id) handles.add(choice.id);
      });
      return handles;
    }
    // Other node types with dynamic source handles can be added here
    default:
      // Return undefined to indicate default behavior (no cleanup needed)
      return undefined;
  }
}

/**
 * Compute the set of valid target handle IDs for a node based on its data.
 * Returns undefined if the node type doesn't have dynamic handles (use default behavior).
 */
export function getValidTargetHandles(data: WorkflowNodeData): Set<string> | undefined {
  // Currently, no node type has dynamic target handles
  // Add cases here if needed in the future
  switch (data.type) {
    default:
      return undefined;
  }
}

/**
 * Check if a node type has dynamic handles that may require edge cleanup.
 */
export function hasDynamicHandles(nodeType: WorkflowNodeData['type']): boolean {
  return nodeType === 'if-else' || nodeType === 'approval' || nodeType === 'question-answer';
  // Add more node types here as needed
}
