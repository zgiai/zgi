// Central node component (owns layout, theming, handles, and dispatches content)
import CustomNode from './custom';
import IterationStartNode from './iteration-start';
import LoopStartNode from './loop-start';
import NoteNode from './note';

// Export central node only (content components are internal to their folders)
export { CustomNode };

// Node types mapping for React Flow — all types use CustomNode
export const nodeTypes = {
  start: CustomNode,
  'knowledge-retrieval': CustomNode,
  llm: CustomNode,
  'http-request': CustomNode,
  'call-database': CustomNode,
  'sql-generator': CustomNode,
  'create-scheduled-task': CustomNode,
  'notification-sms': CustomNode,
  tools: CustomNode,
  end: CustomNode,
  'loop-end': CustomNode,
  'if-else': CustomNode,
  code: CustomNode,
  loop: CustomNode,
  iteration: CustomNode,
  answer: CustomNode,
  assigner: CustomNode,
  'document-extractor': CustomNode,
  'parameter-extractor': CustomNode,
  'variable-aggregator': CustomNode,
  'json-parser': CustomNode,
  'image-gen': CustomNode,
  approval: CustomNode,
  announcement: CustomNode,
  'question-answer': CustomNode,
  'custom-iteration-start': IterationStartNode,
  'custom-loop-start': LoopStartNode,
  custom: CustomNode,
  note: NoteNode,
};

// Node type definitions
export const NODE_TYPES = {
  START: 'start',
  KNOWLEDGE_RETRIEVAL: 'knowledge-retrieval',
  LLM: 'llm',
  HTTP_REQUEST: 'http-request',
  CALL_DATABASE: 'call-database',
  SQL_GENERATOR: 'sql-generator',
  CREATE_SCHEDULED_TASK: 'create-scheduled-task',
  NOTIFICATION_SMS: 'notification-sms',
  TOOL: 'tools',
  END: 'end',
  LOOP_END: 'loop-end',
  ANSWER: 'answer',
  IF_ELSE: 'if-else',
  CODE: 'code',
  LOOP: 'loop',
  ITERATION: 'iteration',
  ITERATION_START: 'iteration-start',
  LOOP_START: 'loop-start',
  ASSIGNER: 'assigner',
  DOCUMENT_EXTRACTOR: 'document-extractor',
  PARAMETER_EXTRACTOR: 'parameter-extractor',
  VARIABLE_AGGREGATOR: 'variable-aggregator',
  JSON_PARSER: 'json-parser',
  IMAGE_GEN: 'image-gen',
  APPROVAL: 'approval',
  ANNOUNCEMENT: 'announcement',
  QUESTION_ANSWER: 'question-answer',
  NOTE: 'note',
} as const;

export type NodeType = (typeof NODE_TYPES)[keyof typeof NODE_TYPES];
