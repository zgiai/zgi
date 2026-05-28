import type { Edge, Viewport, Node } from '@xyflow/react';
import type { NodesSuffix, AllTranslationKeys } from '@/i18n';
export type { NodesSuffix, AllTranslationKeys };
import type { InputVar } from '../types/input-var';
import type { StartNodeData } from '../nodes/start/config';
import type { KnowledgeRetrievalNodeData } from '../nodes/knowledge-retrieval/config';
import type { LLMNodeData } from '../nodes/llm/config';
import type { HttpRequestNodeData } from '../nodes/http-request/config';
import type { EndNodeData } from '../nodes/end/config';
import type { AnswerNodeData } from '../nodes/answer/config';
import type { IterationNodeData } from '../nodes/iteration/config';
import type { CodeNodeData } from '../nodes/code/config';
import type { AssignerNodeData } from '../nodes/assigner/config';
import type { ToolNodeData } from '../nodes/tool/config';
import type { CallDatabaseNodeData } from '../nodes/call-database/config';
import type { SqlGeneratorNodeData } from '../nodes/sql-generator/config';
import type { CreateScheduledTaskNodeData } from '../nodes/create-scheduled-task/config';
import type { NotificationSMSNodeData } from '../nodes/notification-sms/config';
import type { DocumentExtractorNodeData } from '../nodes/document-extractor/config';
import type { ParameterExtractorNodeData } from '../nodes/parameter-extractor/config';
import type { VariableAggregatorNodeData } from '../nodes/variable-aggregator/config';
import type { LoopEndNodeData } from '../nodes/loop-end/config';
import type { JsonParserNodeData } from '../nodes/json-parser/config';
import type { ImageGenNodeData } from '../nodes/image-gen/config';
import type { ApprovalNodeData } from '../nodes/approval/config';
import type { AnnouncementNodeData } from '../nodes/announcement/config';
import type { QuestionAnswerNodeData } from '../nodes/question-answer/config';
import type { IterationStartNodeData } from '../nodes/iteration-start';
import type { LoopStartNodeData } from '../nodes/loop-start';
import type { NodeProps } from '@xyflow/react';
import type { NoteNodeData } from '../nodes/note';
import type { LoopNodeData } from '../nodes/loop/config';
import type { IfElseNodeData } from '../nodes/if-else/types';
export interface WorkflowVariable {
  id: string;
  name: string;
  // Extended primitive types with refined array subtypes and file
  // Keep legacy 'array' for backward compatibility when loading older drafts
  type:
    | 'string'
    | 'number'
    | 'boolean'
    | 'object'
    | 'file'
    | 'array'
    | 'array[string]'
    | 'array[number]'
    | 'array[boolean]'
    | 'array[object]'
    | 'array[file]'
    | 'array[array]';
  value?: unknown;
  description?: string;
}

export interface StoreValidationError {
  type: 'error' | 'warning';
  code: NodesSuffix; // The i18n key
  nodeId?: string;
  nodeTitle?: string;
  params?: Record<string, string | number>; // For i18n interpolation
}

export interface StoreValidationResults {
  errors: StoreValidationError[];
  warnings: StoreValidationError[];
  errorMap: Map<string, StoreValidationError[]>;
  warningMap: Map<string, StoreValidationError[]>;
}

export interface VariableSelector {
  variable: string;
  value_selector: string[];
}

export type WorkflowDragPreviewKind = 'card' | 'container' | 'branch';

export interface WorkflowNodePreviewConfig {
  kind?: WorkflowDragPreviewKind;
  width?: number;
  height?: number;
}

export interface WorkflowDragPreview {
  type: string;
  title: string;
  description?: string;
  iconUrl?: string;
  data?: WorkflowNodeData;
  toolInfo?: {
    provider_id: string;
    tool_name: string;
  };
  client: { x: number; y: number };
  anchor: { x: number; y: number };
}

// Assigner node enums and types
export type {
  WriteMode,
  AssignerNodeInputType,
  AssignerNodeOperation,
} from '../nodes/assigner/config';

// Lightweight JSON Schema types used for structured output
export interface JSONSchemaString {
  type: 'string';
  description?: string;
}
export interface JSONSchemaNumber {
  type: 'number';
  description?: string;
}
export interface JSONSchemaBoolean {
  type: 'boolean';
  description?: string;
}
export interface JSONSchemaArray {
  type: 'array';
  items: JSONSchema;
  description?: string;
}
export interface JSONSchemaObject {
  type: 'object';
  additionalProperties: boolean;
  required: string[];
  properties: Record<string, JSONSchema>;
  description?: string;
}
export type JSONSchema =
  | JSONSchemaString
  | JSONSchemaNumber
  | JSONSchemaBoolean
  | JSONSchemaArray
  | JSONSchemaObject;

// Prompt variable configuration
export interface PromptVariableSelector {
  variable: string;
  value_selector: string[];
  required: boolean;
}
export interface PromptConfig {
  jinja2_variables: PromptVariableSelector[];
}

// Common node type with shared properties
export interface CommonNodeType {
  id: string;
  type: string;
  position: { x: number; y: number };
  data: WorkflowNodeData;
  selected?: boolean;
  sourcePosition?: 'top' | 'right' | 'bottom' | 'left';
  targetPosition?: 'top' | 'right' | 'bottom' | 'left';
  width?: number;
  height?: number;
  _runningStatus?: 'idle' | 'running' | 'completed' | 'failed';
  _singleRunningStatus?: 'idle' | 'running' | 'completed' | 'failed';
  _connectedSourceHandleIds?: string[];
  _connectedTargetHandleIds?: string[];
  _holdAddVariablePopup?: boolean;
}

// Start node data type
export type {
  StartNodeData,
  KnowledgeRetrievalNodeData,
  LLMNodeData,
  HttpRequestNodeData,
  EndNodeData,
  LoopEndNodeData,
  AnswerNodeData,
  IfElseNodeData,
  IterationNodeData,
  IterationStartNodeData,
  LoopNodeData,
  LoopStartNodeData,
  CodeNodeData,
  ToolNodeData,
  AssignerNodeData,
  CallDatabaseNodeData,
  SqlGeneratorNodeData,
  CreateScheduledTaskNodeData,
  NotificationSMSNodeData,
  DocumentExtractorNodeData,
  ParameterExtractorNodeData,
  VariableAggregatorNodeData,
  JsonParserNodeData,
  ImageGenNodeData,
  ApprovalNodeData,
  AnnouncementNodeData,
  QuestionAnswerNodeData,
  NoteNodeData,
};

// Start node type extending common properties
export type StartNodeType = CommonNodeType & { data: StartNodeData; variables: InputVar[] };
export type { OutputVariable } from '../nodes/end/config';

// Http Request node data type
export type { HttpHeaderKV } from '../nodes/http-request/config';

// JSON types for future structured handling
export type JSONPrimitive = string | number | boolean | null;
export type JSONValue = JSONPrimitive | JSONValue[] | { [key: string]: JSONValue };

// Discriminated union for HTTP request body
// Updated schema: support none/raw-text/json with data array items
export type { HttpRequestBodyDataItem, HttpRequestBody } from '../nodes/http-request/config';

// Iteration node data type
export type {
  IterationPrimitiveType,
  IterationValueSelector,
  ErrorHandleMode,
} from '../nodes/iteration/config';

export type WorkflowNodeData =
  | StartNodeData
  | KnowledgeRetrievalNodeData
  | LLMNodeData
  | HttpRequestNodeData
  | EndNodeData
  | LoopEndNodeData
  | AnswerNodeData
  | IfElseNodeData
  | IterationNodeData
  | IterationStartNodeData
  | LoopNodeData
  | LoopStartNodeData
  | CodeNodeData
  | ToolNodeData
  | AssignerNodeData
  | CallDatabaseNodeData
  | SqlGeneratorNodeData
  | CreateScheduledTaskNodeData
  | NotificationSMSNodeData
  | DocumentExtractorNodeData
  | ParameterExtractorNodeData
  | VariableAggregatorNodeData
  | JsonParserNodeData
  | ImageGenNodeData
  | ApprovalNodeData
  | AnnouncementNodeData
  | QuestionAnswerNodeData
  | NoteNodeData;

export interface ContainerBaseData {
  start_node_id: string;
  _children: string[];
}

export const CONTAINER_NODE_TYPES = ['iteration', 'loop'] as const;
export type ContainerNodeType = (typeof CONTAINER_NODE_TYPES)[number];

export const isContainerNode = (type?: string): type is ContainerNodeType => {
  return (CONTAINER_NODE_TYPES as readonly string[]).includes(type || '');
};

export const isContainerStartNode = (type?: string): boolean => {
  // Currently we use a convention where container start nodes end with '-start'
  return type?.endsWith('-start') || type === 'iteration-start' || type === 'loop-start';
};

export const getContainerStartType = (containerType: string) =>
  `${containerType}-start` as ContainerNodeType;
export const getContainerCustomStartType = (containerType: string) =>
  `custom-${containerType}-start`;

// Satisfy @xyflow/react Node generic constraint (Data extends Record<string, unknown>)
export type WorkflowNode = Node<WorkflowNodeData & Record<string, unknown>>;

export type WorkflowEdge = Edge<
  {
    sourceType: string;
    targetType: string;
    isInLoop: boolean;
    desc?: string;
  } & Record<string, unknown>
>;

// Strict unions for file upload configuration
export type FileUploadType = 'image' | 'audio' | 'video' | 'document' | 'custom';
export type FileUploadMethod = 'local_file' | 'remote_url';

export interface WebAppWorkflowConfigFeature {
  allow_view_run_detail: boolean;
  auto_expand_run_detail: boolean;
}

export interface WorkflowFeatures {
  opening_statement_type: 'slogan' | 'message';
  opening_guide_version?: 2;
  opening_slogan: string;
  opening_statement: string;
  opening_statement_enabled: boolean;
  suggested_questions: string[];
  suggested_questions_after_answer: {
    enabled: boolean;
  };
  text_to_speech: {
    enabled: boolean;
    voice: string;
    language: string;
  };
  speech_to_text: {
    enabled: boolean;
  };
  retriever_resource: {
    enabled: boolean;
  };
  sensitive_word_avoidance: {
    enabled: boolean;
  };
  conversation_history: {
    enabled: boolean;
    history_window_size: number;
  };
  file_upload: {
    enabled: boolean;
    allowed_file_types: FileUploadType[];
    allowed_file_extensions: string[];
    allowed_file_upload_methods: FileUploadMethod[];
    number_limits: number;
  };
  webapp_workflow_config: WebAppWorkflowConfigFeature;
}

export interface EnvironmentVariable {
  id: string;
  name: string;
  // Environment variable primitive type
  // - secret behaves like string but will be encrypted by backend after first save
  type: 'string' | 'number' | 'secret';
  // Raw value for string/number/secret (secret editable only before first save)
  value: string;
  description?: string;
  // When true and type is 'secret', the value is masked and not editable
  is_secret_set?: boolean;
}

export interface ConversationVariable {
  id: string;
  name: string;
  // Match WorkflowVariable primitive union, including refined array subtypes and file
  type:
    | 'string'
    | 'number'
    | 'boolean'
    | 'object'
    | 'array[string]'
    | 'array[number]'
    | 'array[boolean]'
    | 'array[object]';
  value?: unknown;
  description?: string;
}

// Draft-only item shape for conversation variables
export interface ConversationVariableDraftItem {
  id: string;
  name: string;
  value_type:
    | 'string'
    | 'number'
    | 'boolean'
    | 'object'
    | 'array[string]'
    | 'array[number]'
    | 'array[boolean]'
    | 'array[object]';
  value?: unknown;
  description?: string;
}

// Payload shape for saving workflow draft to server
export interface WorkflowDraftSavePayload {
  graph: WorkflowData['graph'];
  features: WorkflowData['features'];
  environment_variables: EnvironmentVariable[];
  conversation_variables: ConversationVariableDraftItem[];
  hash: string;
}

export interface WorkflowData {
  graph: {
    nodes: WorkflowNode[];
    edges: WorkflowEdge[];
    viewport: Viewport;
  };
  features: WorkflowFeatures;
  environment_variables: EnvironmentVariable[];
  conversation_variables: ConversationVariable[];
  hash: string;
}

/**
 * Workflow draft data structure returned from API
 */
export interface WorkflowDraftData {
  id: string;
  agent_id: string;
  tenant_id: string;
  type: 'workflow';
  version: 'draft';
  graph: {
    nodes: WorkflowNode[];
    edges: WorkflowEdge[];
    viewport: Viewport;
  };
  features: {
    annotation_reply: {
      enabled: boolean;
    };
    // Draft API may provide either a legacy nested image config or the flat config
    file_upload?: {
      // New flat shape (optional in drafts)
      enabled?: boolean;
      allowed_file_types?: FileUploadType[];
      allowed_file_extensions?: string[];
      allowed_file_upload_methods?: FileUploadMethod[];
      number_limits?: number;
      // Legacy nested shape for backward compatibility
      image?: {
        detail?: string;
        enabled?: boolean;
        number_limits?: number;
        transfer_methods?: FileUploadMethod[];
      };
    };
    opening_statement_type?: 'slogan' | 'message';
    opening_guide_version?: 2;
    opening_slogan?: string;
    opening_statement: string;
    opening_statement_enabled?: boolean;
    retriever_resource: {
      enabled: boolean;
    };
    speech_to_text: {
      enabled: boolean;
    };
    suggested_questions: string[];
    suggested_questions_after_answer: {
      enabled: boolean;
    };
    text_to_speech: {
      enabled: boolean;
    };
    conversation_history?: {
      enabled: boolean;
      history_window_size: number;
    };
    webapp_workflow_config?: WebAppWorkflowConfigFeature;
  };
  environment_variables: Record<string, unknown>;
  // Draft stores conversation variables as an array with value_type
  conversation_variables: ConversationVariableDraftItem[];
  created_at: number;
  updated_at: number;
  created_by: string;
  hash?: string;
}

// Global helpers for React Flow generics compatibility (Node/Data/Props)
// Keep data model clean while satisfying RF's Record<string, unknown> constraint

export type NodeDataCompat<T> = T & Record<string, unknown>;
export type NodeCompat<TData, TType extends string | undefined = string | undefined> = Node<
  NodeDataCompat<TData>,
  TType
>;
export type NodePropsCompat<
  TData,
  TType extends string | undefined = string | undefined,
> = NodeProps<NodeCompat<TData, TType>>;
