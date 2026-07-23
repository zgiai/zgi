import type { WorkflowTestCase, WorkflowTestTurn } from '@/services/types/workflow-test';

export type WorkflowTestMode = 'task' | 'conversation';

export type ExpectedCheckType = 'node' | 'capability' | 'output_contains' | 'latency';
export type ExpectedCheckSeverity = 'critical' | 'normal' | 'hint';
export type ExpectedCheckSource = 'ai_generated' | 'user_added' | 'system_default';
export type ExpectedCheckMatchMode = 'semantic' | 'keyword';
export type ConversationCheckType =
  | 'intent_understanding'
  | 'context_following'
  | 'memory'
  | 'clarification'
  | 'output_format'
  | 'fallback'
  | 'reply_contains'
  | 'safety'
  | 'task_completion'
  | 'consistency'
  | 'no_hallucination'
  | 'no_system_leak'
  | 'tone';

export interface ExpectedCheckCondition {
  id: string;
  type: ExpectedCheckType;
  operator: string;
  target_id?: string;
  target_label?: string;
  target_type?: string;
  values?: string[];
  match_mode?: ExpectedCheckMatchMode;
  value_ms?: number | string;
  severity?: ExpectedCheckSeverity;
  source?: ExpectedCheckSource;
}

export interface ExpectedChecks {
  must_visit_nodes?: string[];
  must_call_tools?: string[];
  output_contains?: string[];
  max_latency_ms?: number | string;
  conditions?: ExpectedCheckCondition[];
}

export interface ConversationCheckCondition {
  id: string;
  type: ConversationCheckType;
  operator: string;
  values?: string[];
  match_mode?: ExpectedCheckMatchMode;
  severity?: ExpectedCheckSeverity;
  source?: ExpectedCheckSource;
}

export interface ConversationChecks {
  conditions?: ConversationCheckCondition[];
}

export interface WorkflowCheckOption {
  id: string;
  label: string;
  type: string;
  description?: string;
  node_id?: string;
  node_label?: string;
}

export interface WorkflowInputVariableOption {
  key: string;
  label: string;
  type: string;
  required?: boolean;
  description?: string;
  stale?: boolean;
}

export interface GeneratedFixtureSpec {
  name: string;
  format: string;
  title: string;
  facts: string[];
  expected_checks: string[];
}

const DEFAULT_ATTACHMENT_EXTENSIONS_BY_TYPE: Record<string, string[]> = {
  image: ['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg'],
  document: ['pdf', 'doc', 'docx', 'xls', 'xlsx', 'csv', 'txt', 'md', 'json'],
  audio: ['mp3', 'wav', 'm4a', 'aac', 'ogg'],
  video: ['mp4', 'mov', 'avi', 'mkv', 'webm'],
};

const CHECKABLE_NODE_TYPES = new Set([
  'start',
  'knowledge-retrieval',
  'llm',
  'http-request',
  'httprequest',
  'call-database',
  'sql-generator',
  'create-scheduled-task',
  'notification-sms',
  'tool',
  'tools',
  'end',
  'answer',
  'if-else',
  'code',
  'loop',
  'iteration',
  'assigner',
  'document-extractor',
  'document_extractor',
  'parameter-extractor',
  'parameter_extractor',
  'variable-aggregator',
  'variable_aggregator',
  'json-parser',
  'json_parser',
  'image-gen',
  'image_gen',
  'approval',
  'announcement',
  'question-answer',
  'question_answer',
]);

const CAPABILITY_NODE_TYPES = new Set([
  'tool',
  'tools',
  'http-request',
  'httprequest',
  'code',
  'question-answer',
  'question_answer',
  'document-extractor',
  'document_extractor',
  'knowledge-retrieval',
  'knowledge_retrieval',
  'call-database',
  'call_database',
  'sql-generator',
  'sql_generator',
  'image-gen',
  'image_gen',
]);

export const CASE_MODE_KEY = '__case_mode';
export const CASE_TAGS_KEY = '__tags';
export const EXPECTED_CHECKS_KEY = '__expected_checks';
export const TURN_EXPECTATION_KEY = '__turn_expectation';
export const TURN_CHECKS_KEY = '__turn_checks';
export const CONVERSATION_CHECKS_KEY = '__conversation_checks';
export const ASSET_SOURCE_KEY = '__asset_source';
export const FIXTURE_SPEC_KEY = '__fixture_spec';
export const GENERATED_ASSET_SOURCE = 'workflow_test_generated';

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value));
}

function asStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.map(item => String(item ?? '').trim()).filter(Boolean);
}

function asString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function asBoolean(value: unknown): boolean | undefined {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase();
    if (normalized === 'true') return true;
    if (normalized === 'false') return false;
  }
  return undefined;
}

function getNodeData(node: unknown): Record<string, unknown> {
  if (!isRecord(node)) return {};
  return isRecord(node.data) ? node.data : {};
}

function getNodeDataString(node: unknown, key: string): string {
  const data = getNodeData(node);
  return asString(data[key]);
}

function normalizeNodeType(type: string): string {
  return type.trim().toLowerCase();
}

function isFileInputType(type: unknown) {
  const normalized = asString(type);
  return normalized === 'file' || normalized === 'file-list' || normalized === 'array[file]';
}

function normalizeExtension(value: unknown): string {
  return asString(value).toLowerCase().replace(/^\./, '');
}

function extensionList(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.map(normalizeExtension).filter(Boolean);
}

function attachmentTypesToExtensions(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.flatMap(
    type => DEFAULT_ATTACHMENT_EXTENSIONS_BY_TYPE[asString(type).toLowerCase()] ?? []
  );
}

function getDraftFeatures(draft: unknown): Record<string, unknown> {
  return isRecord(draft) && isRecord(draft.features) ? draft.features : {};
}

function getDraftNodes(draft: unknown): unknown[] {
  if (!isRecord(draft) || !isRecord(draft.graph) || !Array.isArray(draft.graph.nodes)) return [];
  return draft.graph.nodes;
}

function getDraftEdges(draft: unknown): unknown[] {
  if (!isRecord(draft) || !isRecord(draft.graph) || !Array.isArray(draft.graph.edges)) return [];
  return draft.graph.edges;
}

function startNodeVariables(draft: unknown): unknown[] {
  const startNode = getDraftNodes(draft).find(node => {
    if (!isRecord(node)) return false;
    return normalizeNodeType(getNodeDataString(node, 'type') || asString(node.type)) === 'start';
  });
  const variables = getNodeData(startNode).variables;
  return Array.isArray(variables) ? variables : [];
}

function selectorEquals(value: unknown, expected: string[]) {
  const selector = asStringArray(value);
  if (selector.length < expected.length) return false;
  return expected.every((part, index) => selector[index] === part);
}

function draftHasSystemFilesConsumer(draft: unknown): boolean {
  return getDraftNodes(draft).some(node => {
    const data = getNodeData(node);
    const type = normalizeNodeType(
      asString(data.type) || (isRecord(node) ? asString(node.type) : '')
    );
    if (type === 'document-extractor' || type === 'document_extractor') {
      return selectorEquals(data.variable_selector, ['sys', 'files']);
    }
    if (type === 'llm') {
      const vision = isRecord(data.vision) ? data.vision : {};
      const configs = isRecord(vision.configs) ? vision.configs : {};
      return selectorEquals(configs.variable_selector, ['sys', 'files']);
    }
    return false;
  });
}

function createConditionId(index: number, type: ExpectedCheckType) {
  return `check_${type}_${index + 1}`;
}

function createConversationConditionId(index: number, type: ConversationCheckType) {
  return `conversation_check_${type}_${index + 1}`;
}

function normalizeCondition(raw: unknown, index: number): ExpectedCheckCondition | null {
  if (!isRecord(raw)) return null;
  const type = asString(raw.type) as ExpectedCheckType;
  if (!['node', 'capability', 'output_contains', 'latency'].includes(type)) return null;
  const condition: ExpectedCheckCondition = {
    id: asString(raw.id) || createConditionId(index, type),
    type,
    operator: asString(raw.operator) || defaultOperatorForCheckType(type),
    severity: ['critical', 'normal', 'hint'].includes(asString(raw.severity))
      ? (asString(raw.severity) as ExpectedCheckSeverity)
      : 'normal',
    source: ['ai_generated', 'user_added', 'system_default'].includes(asString(raw.source))
      ? (asString(raw.source) as ExpectedCheckSource)
      : 'user_added',
  };
  const targetID = asString(raw.target_id);
  const targetLabel = asString(raw.target_label);
  const targetType = asString(raw.target_type);
  if (targetID) condition.target_id = targetID;
  if (targetLabel) condition.target_label = targetLabel;
  if (targetType) condition.target_type = targetType;
  const values = asStringArray(raw.values);
  if (values.length > 0) condition.values = values;
  if (raw.match_mode === 'semantic' || raw.match_mode === 'keyword') {
    condition.match_mode = raw.match_mode;
  }
  if (typeof raw.value_ms === 'number' || typeof raw.value_ms === 'string') {
    condition.value_ms = raw.value_ms;
  }
  return condition;
}

function normalizeConversationCondition(
  raw: unknown,
  index: number
): ConversationCheckCondition | null {
  if (!isRecord(raw)) return null;
  const type = asString(raw.type) as ConversationCheckType;
  if (!conversationCheckTypes().includes(type)) return null;
  const condition: ConversationCheckCondition = {
    id: asString(raw.id) || createConversationConditionId(index, type),
    type,
    operator: asString(raw.operator) || defaultOperatorForConversationCheckType(type),
    severity: ['critical', 'normal', 'hint'].includes(asString(raw.severity))
      ? (asString(raw.severity) as ExpectedCheckSeverity)
      : 'normal',
    source: ['ai_generated', 'user_added', 'system_default'].includes(asString(raw.source))
      ? (asString(raw.source) as ExpectedCheckSource)
      : 'user_added',
  };
  const values = asStringArray(raw.values);
  if (values.length > 0) condition.values = values;
  if (raw.match_mode === 'semantic' || raw.match_mode === 'keyword') {
    condition.match_mode = raw.match_mode;
  }
  return condition;
}

function legacyChecksToConditions(raw: Record<string, unknown>): ExpectedCheckCondition[] {
  const conditions: ExpectedCheckCondition[] = [];
  asStringArray(raw.must_visit_nodes).forEach(value => {
    conditions.push({
      id: createConditionId(conditions.length, 'node'),
      type: 'node',
      operator: 'visited',
      target_id: value,
      target_label: value,
      severity: 'normal',
      source: 'user_added',
    });
  });
  asStringArray(raw.must_call_tools).forEach(value => {
    conditions.push({
      id: createConditionId(conditions.length, 'capability'),
      type: 'capability',
      operator: 'called',
      target_id: value,
      target_label: value,
      severity: 'normal',
      source: 'user_added',
    });
  });
  const outputValues = asStringArray(raw.output_contains);
  if (outputValues.length > 0) {
    conditions.push({
      id: createConditionId(conditions.length, 'output_contains'),
      type: 'output_contains',
      operator: 'contains',
      values: outputValues,
      match_mode: 'semantic',
      severity: 'normal',
      source: 'user_added',
    });
  }
  if (typeof raw.max_latency_ms === 'number' || typeof raw.max_latency_ms === 'string') {
    conditions.push({
      id: createConditionId(conditions.length, 'latency'),
      type: 'latency',
      operator: 'lte',
      value_ms: raw.max_latency_ms,
      severity: 'hint',
      source: 'user_added',
    });
  }
  return conditions;
}

function capabilityOptionForNode(node: unknown): WorkflowCheckOption | null {
  if (!isRecord(node)) return null;
  const id = asString(node.id);
  if (!id) return null;
  const data = getNodeData(node);
  const type = normalizeNodeType(asString(data.type) || asString(node.type));
  if (!CAPABILITY_NODE_TYPES.has(type)) return null;
  const title = asString(data.title) || asString(data.name) || type || id;
  if (type === 'tool' || type === 'tools') {
    return {
      id,
      label: asString(data.tool_name) || asString(data.name) || title,
      type,
      description: title,
      node_id: id,
      node_label: title,
    };
  }
  if (type === 'httprequest' || type === 'http-request') {
    const method = asString(data.method);
    const url = asString(data.url);
    return {
      id,
      label: method || url ? [method, url].filter(Boolean).join(' ') : title,
      type,
      description: title,
      node_id: id,
      node_label: title,
    };
  }
  return {
    id,
    label: title,
    type,
    node_id: id,
    node_label: title,
  };
}

export function defaultOperatorForCheckType(type: ExpectedCheckType): string {
  if (type === 'node') return 'visited';
  if (type === 'capability') return 'called';
  if (type === 'latency') return 'lte';
  return 'contains';
}

export function createExpectedCheckCondition(
  type: ExpectedCheckType = 'output_contains'
): ExpectedCheckCondition {
  return {
    id: globalThis.crypto?.randomUUID?.() ?? `check_${Date.now()}_${Math.random()}`,
    type,
    operator: defaultOperatorForCheckType(type),
    values: type === 'output_contains' ? [] : undefined,
    match_mode: type === 'output_contains' ? 'semantic' : undefined,
    severity: type === 'latency' ? 'hint' : 'normal',
    source: 'user_added',
  };
}

export function conversationCheckTypes(): ConversationCheckType[] {
  return [
    'intent_understanding',
    'context_following',
    'memory',
    'clarification',
    'output_format',
    'fallback',
    'reply_contains',
    'safety',
    'task_completion',
    'consistency',
    'no_hallucination',
    'no_system_leak',
    'tone',
  ];
}

export function defaultOperatorForConversationCheckType(type: ConversationCheckType): string {
  if (type === 'reply_contains') return 'contains';
  if (type === 'no_hallucination' || type === 'no_system_leak') return 'passed';
  return 'passed';
}

export function createConversationCheckCondition(
  type: ConversationCheckType = 'intent_understanding'
): ConversationCheckCondition {
  return {
    id: globalThis.crypto?.randomUUID?.() ?? `conversation_check_${Date.now()}_${Math.random()}`,
    type,
    operator: defaultOperatorForConversationCheckType(type),
    values: [],
    match_mode: 'semantic',
    severity: 'normal',
    source: 'user_added',
  };
}

export function workflowDraftMode(draft: unknown): WorkflowTestMode {
  const type =
    isRecord(draft) && typeof draft.type === 'string'
      ? draft.type
      : isRecord(draft) && typeof draft.workflow_type === 'string'
        ? draft.workflow_type
        : '';
  return type.trim().toLowerCase() === 'chat' ? 'conversation' : 'task';
}

export function draftSupportsFileInputs(draft: unknown): boolean {
  const mode = workflowDraftMode(draft);
  if (mode === 'conversation') {
    const features = getDraftFeatures(draft);
    const fileUpload = isRecord(features.file_upload) ? features.file_upload : {};
    const legacyImageUpload = isRecord(fileUpload.image) ? fileUpload.image : {};
    return Boolean(
      asBoolean(fileUpload.enabled) ||
        asBoolean(legacyImageUpload.enabled) ||
        draftHasSystemFilesConsumer(draft)
    );
  }
  const variables = startNodeVariables(draft);
  if (variables.length === 0) return false;
  return variables.some(variable => isRecord(variable) && isFileInputType(variable.type));
}

export function draftRequiresCurrentTurnFiles(draft: unknown): boolean {
  const nodes = getDraftNodes(draft);
  if (nodes.length === 0) return false;

  const nodeRequiresFiles = new Map<string, boolean>();
  const knownNodeIds = new Set<string>();
  const startNodeIds: string[] = [];
  nodes.forEach(node => {
    if (!isRecord(node)) return;
    const id = asString(node.id);
    const data = getNodeData(node);
    const type = normalizeNodeType(asString(data.type) || asString(node.type));
    if (!id || type === 'note' || type === 'comment') return;
    knownNodeIds.add(id);
    if (type === 'start') startNodeIds.push(id);
    if (type === 'document-extractor' || type === 'document_extractor') {
      nodeRequiresFiles.set(id, selectorEquals(data.variable_selector, ['sys', 'files']));
    }
  });
  if (startNodeIds.length === 0) return false;

  const children = new Map<string, string[]>();
  getDraftEdges(draft).forEach(edge => {
    if (!isRecord(edge)) return;
    const source = asString(edge.source);
    const target = asString(edge.target);
    if (!knownNodeIds.has(source) || !knownNodeIds.has(target)) return;
    children.set(source, [...(children.get(source) ?? []), target]);
  });

  const walk = (
    nodeId: string,
    filesSeen: boolean,
    visiting: Set<string>
  ): { hasTerminal: boolean; allRequireFiles: boolean } => {
    if (visiting.has(nodeId)) return { hasTerminal: false, allRequireFiles: false };
    const nextFilesSeen = filesSeen || nodeRequiresFiles.get(nodeId) === true;
    const next = children.get(nodeId) ?? [];
    if (next.length === 0) return { hasTerminal: true, allRequireFiles: nextFilesSeen };
    const nextVisiting = new Set(visiting).add(nodeId);
    const results = next.map(target => walk(target, nextFilesSeen, nextVisiting));
    const terminalResults = results.filter(result => result.hasTerminal);
    if (terminalResults.length === 0) return { hasTerminal: false, allRequireFiles: false };
    return {
      hasTerminal: true,
      allRequireFiles: terminalResults.every(result => result.allRequireFiles),
    };
  };

  return startNodeIds.every(startNodeId => {
    const result = walk(startNodeId, false, new Set());
    return result.hasTerminal && result.allRequireFiles;
  });
}

export function draftAttachmentAcceptExtensions(draft: unknown): string[] {
  const mode = workflowDraftMode(draft);
  if (mode === 'conversation') {
    const features = getDraftFeatures(draft);
    const fileUpload = isRecord(features.file_upload) ? features.file_upload : {};
    const explicitExtensions = extensionList(fileUpload.allowed_file_extensions);
    if (explicitExtensions.length > 0) return explicitExtensions;
    const fromTypes = attachmentTypesToExtensions(fileUpload.allowed_file_types);
    if (fromTypes.length > 0) return Array.from(new Set(fromTypes));
    if (draftHasSystemFilesConsumer(draft)) {
      return DEFAULT_ATTACHMENT_EXTENSIONS_BY_TYPE.document;
    }
    return [];
  }
  const variables = startNodeVariables(draft).filter(
    variable => isRecord(variable) && isFileInputType(variable.type)
  ) as Array<Record<string, unknown>>;
  const explicitExtensions = variables.flatMap(variable =>
    extensionList(variable.allowed_file_extensions ?? variable.allowed_extensions)
  );
  if (explicitExtensions.length > 0) return Array.from(new Set(explicitExtensions));
  const fromTypes = variables.flatMap(variable =>
    attachmentTypesToExtensions(variable.allowed_file_types ?? variable.file_types)
  );
  return Array.from(new Set(fromTypes));
}

export function turnInputs(turn?: WorkflowTestTurn): Record<string, unknown> {
  return isRecord(turn?.inputs) ? turn.inputs : {};
}

export function caseModeFromCase(
  item: WorkflowTestCase,
  fallback: WorkflowTestMode
): WorkflowTestMode {
  const mode = turnInputs(item.turns?.[0])[CASE_MODE_KEY];
  return mode === 'task' || mode === 'conversation' ? mode : fallback;
}

export function caseTags(item: WorkflowTestCase): string[] {
  return asStringArray(turnInputs(item.turns?.[0])[CASE_TAGS_KEY]);
}

export function hasGeneratedAssets(item: WorkflowTestCase): boolean {
  return turnInputs(item.turns?.[0])[ASSET_SOURCE_KEY] === GENERATED_ASSET_SOURCE;
}

export function fixtureSpecsFromInputs(inputs: Record<string, unknown>): GeneratedFixtureSpec[] {
  const raw = inputs[FIXTURE_SPEC_KEY];
  if (!Array.isArray(raw)) return [];
  return raw
    .map((item): GeneratedFixtureSpec | null => {
      if (!isRecord(item)) return null;
      const fixture = isRecord(item.fixture) ? item.fixture : {};
      return {
        name: asString(item.name) || asString(fixture.filename) || asString(fixture.title),
        format: asString(item.format) || asString(fixture.format),
        title: asString(fixture.title),
        facts: asStringArray(fixture.facts),
        expected_checks: asStringArray(fixture.expected_checks),
      };
    })
    .filter((item): item is GeneratedFixtureSpec =>
      Boolean(
        item &&
          (item.name || item.title || item.facts.length > 0 || item.expected_checks.length > 0)
      )
    );
}

export function generatedFixtureSpecs(item: WorkflowTestCase): GeneratedFixtureSpec[] {
  return fixtureSpecsFromInputs(turnInputs(item.turns?.[0]));
}

export function expectedChecks(item: WorkflowTestCase): ExpectedChecks {
  const raw = turnInputs(item.turns?.[0])[EXPECTED_CHECKS_KEY];
  return normalizeExpectedChecks(raw);
}

export function normalizeExpectedChecks(raw: unknown): ExpectedChecks {
  if (!isRecord(raw)) return {};
  const conditions = Array.isArray(raw.conditions)
    ? raw.conditions
        .map((item, index) => normalizeCondition(item, index))
        .filter((item): item is ExpectedCheckCondition => Boolean(item))
    : legacyChecksToConditions(raw);
  return {
    must_visit_nodes: asStringArray(raw.must_visit_nodes),
    must_call_tools: asStringArray(raw.must_call_tools),
    output_contains: asStringArray(raw.output_contains),
    max_latency_ms:
      typeof raw.max_latency_ms === 'number' || typeof raw.max_latency_ms === 'string'
        ? raw.max_latency_ms
        : undefined,
    conditions,
  };
}

export function normalizeConversationChecks(
  raw: unknown,
  fallbackExpectation = ''
): ConversationChecks {
  if (isRecord(raw) && Array.isArray(raw.conditions)) {
    return {
      conditions: raw.conditions
        .map((item, index) => normalizeConversationCondition(item, index))
        .filter((item): item is ConversationCheckCondition => Boolean(item)),
    };
  }
  const expectation = fallbackExpectation.trim();
  if (!expectation) return {};
  return {
    conditions: [
      {
        id: createConversationConditionId(0, 'reply_contains'),
        type: 'reply_contains',
        operator: 'contains',
        values: [expectation],
        match_mode: 'semantic',
        severity: 'normal',
        source: 'system_default',
      },
    ],
  };
}

export function turnChecks(turn?: WorkflowTestTurn): ConversationChecks {
  const inputs = turnInputs(turn);
  return normalizeConversationChecks(inputs[TURN_CHECKS_KEY], turnExpectation(turn));
}

export function conversationChecks(item: WorkflowTestCase): ConversationChecks {
  return normalizeConversationChecks(turnInputs(item.turns?.[0])[CONVERSATION_CHECKS_KEY]);
}

export function conversationCheckConditionCount(item: WorkflowTestCase): number {
  const turnCheckCount = (item.turns ?? []).reduce(
    (count, turn) => count + (turnChecks(turn).conditions?.length ?? 0),
    0
  );
  return turnCheckCount + (conversationChecks(item).conditions?.length ?? 0);
}

export function buildConversationChecksPayload(
  conditions: ConversationCheckCondition[]
): ConversationChecks {
  const normalized = conditions
    .map((condition, index): ConversationCheckCondition | null => {
      const type = condition.type;
      const values = condition.values?.map(item => item.trim()).filter(Boolean) ?? [];
      const operator = condition.operator || defaultOperatorForConversationCheckType(type);
      if (operator !== 'passed' && values.length === 0) return null;
      if (operator === 'passed' && values.length === 0) return null;
      return {
        id: condition.id || createConversationConditionId(index, type),
        type,
        operator,
        values,
        match_mode: condition.match_mode || 'semantic',
        severity: condition.severity || 'normal',
        source: condition.source || 'user_added',
      };
    })
    .filter((item): item is ConversationCheckCondition => Boolean(item));
  return { conditions: normalized };
}

export function expectedCheckConditionCount(checks: ExpectedChecks): number {
  if (checks.conditions && checks.conditions.length > 0) {
    return checks.conditions.length;
  }
  return (
    (checks.must_visit_nodes?.length ?? 0) +
    (checks.must_call_tools?.length ?? 0) +
    (checks.output_contains?.length ?? 0) +
    (checks.max_latency_ms ? 1 : 0)
  );
}

function normalizeCheckTarget(
  condition: ExpectedCheckCondition,
  options?: { nodeOptions: WorkflowCheckOption[]; capabilityOptions: WorkflowCheckOption[] }
) {
  const sourceOptions =
    condition.type === 'node'
      ? options?.nodeOptions
      : condition.type === 'capability'
        ? options?.capabilityOptions
        : undefined;
  if (!sourceOptions || sourceOptions.length === 0) {
    return condition;
  }
  const targetId = asString(condition.target_id);
  const targetLabel = asString(condition.target_label);
  const matched =
    sourceOptions.find(option => option.id === targetId) ||
    sourceOptions.find(option => option.label === targetLabel) ||
    sourceOptions.find(option => option.id === targetLabel);
  if (!matched) {
    return condition;
  }
  return {
    ...condition,
    target_id: matched.id,
    target_label: matched.label,
    target_type: matched.type,
  };
}

export function buildExpectedChecksPayload(
  conditions: ExpectedCheckCondition[],
  options?: { nodeOptions: WorkflowCheckOption[]; capabilityOptions: WorkflowCheckOption[] }
): ExpectedChecks {
  const normalized = conditions
    .map((condition, index): ExpectedCheckCondition | null => {
      const normalizedTarget = normalizeCheckTarget(condition, options);
      const type = normalizedTarget.type;
      const base: ExpectedCheckCondition = {
        id: normalizedTarget.id || createConditionId(index, type),
        type,
        operator: normalizedTarget.operator || defaultOperatorForCheckType(type),
        severity: normalizedTarget.severity || 'normal',
        source: normalizedTarget.source || 'user_added',
      };
      if (normalizedTarget.target_id) base.target_id = normalizedTarget.target_id;
      if (normalizedTarget.target_label) base.target_label = normalizedTarget.target_label;
      if (normalizedTarget.target_type) base.target_type = normalizedTarget.target_type;
      const values = normalizedTarget.values?.map(item => item.trim()).filter(Boolean) ?? [];
      if (values.length > 0) base.values = values;
      if (normalizedTarget.match_mode) base.match_mode = normalizedTarget.match_mode;
      if (normalizedTarget.value_ms !== undefined && normalizedTarget.value_ms !== '') {
        base.value_ms = normalizedTarget.value_ms;
      }

      if ((type === 'node' || type === 'capability') && !base.target_id && !base.target_label) {
        return null;
      }
      if (
        type === 'node' &&
        ['input_contains', 'input_not_contains', 'output_contains', 'output_not_contains'].includes(
          base.operator
        ) &&
        (!base.values || base.values.length === 0)
      ) {
        return null;
      }
      if (type === 'output_contains' && (!base.values || base.values.length === 0)) return null;
      if (type === 'latency') {
        const latency = Number(base.value_ms);
        if (!Number.isFinite(latency) || latency <= 0) return null;
        base.value_ms = latency;
      }
      return base;
    })
    .filter((item): item is ExpectedCheckCondition => Boolean(item));

  const payload: ExpectedChecks = {
    conditions: normalized,
    must_visit_nodes: normalized
      .filter(condition => condition.type === 'node' && condition.operator === 'visited')
      .map(condition => condition.target_label || condition.target_id || '')
      .filter(Boolean),
    must_call_tools: normalized
      .filter(condition => condition.type === 'capability' && condition.operator === 'called')
      .map(condition => condition.target_label || condition.target_id || '')
      .filter(Boolean),
    output_contains: normalized
      .filter(
        condition => condition.type === 'output_contains' && condition.operator === 'contains'
      )
      .flatMap(condition => condition.values ?? []),
  };
  const latency = normalized.find(
    condition => condition.type === 'latency' && condition.operator === 'lte'
  );
  if (latency?.value_ms !== undefined) {
    payload.max_latency_ms = latency.value_ms;
  }
  return payload;
}

export function buildWorkflowCheckOptions(draft: { graph?: { nodes?: unknown[] } } | undefined): {
  nodeOptions: WorkflowCheckOption[];
  capabilityOptions: WorkflowCheckOption[];
} {
  const nodes = draft?.graph?.nodes;
  if (!Array.isArray(nodes)) {
    return { nodeOptions: [], capabilityOptions: [] };
  }
  const nodeOptions = nodes
    .map((node): WorkflowCheckOption | null => {
      if (!isRecord(node)) return null;
      const id = asString(node.id);
      if (!id) return null;
      const type = normalizeNodeType(getNodeDataString(node, 'type') || asString(node.type));
      if (!CHECKABLE_NODE_TYPES.has(type)) return null;
      const label =
        getNodeDataString(node, 'title') || getNodeDataString(node, 'name') || type || id;
      return { id, label, type };
    })
    .filter((item): item is WorkflowCheckOption => Boolean(item));
  const capabilityOptions = nodes
    .map(capabilityOptionForNode)
    .filter((item): item is WorkflowCheckOption => Boolean(item));
  return { nodeOptions, capabilityOptions };
}

export function buildWorkflowInputVariableOptions(
  draft: { graph?: { nodes?: unknown[] } } | undefined
): WorkflowInputVariableOption[] {
  return startNodeVariables(draft)
    .map((variable): WorkflowInputVariableOption | null => {
      if (!isRecord(variable)) return null;
      const key = asString(variable.variable) || asString(variable.key) || asString(variable.name);
      if (!key || key.startsWith('sys.')) return null;
      const type = asString(variable.type);
      if (type === 'file' || type === 'file-list' || type === 'array[file]') return null;
      return {
        key,
        label: asString(variable.label) || key,
        type: type || 'string',
        required: variable.required === true,
        description: asString(variable.description),
      };
    })
    .filter((item): item is WorkflowInputVariableOption => Boolean(item));
}

export function turnExpectation(turn?: WorkflowTestTurn): string {
  const value = turnInputs(turn)[TURN_EXPECTATION_KEY];
  return typeof value === 'string' ? value : '';
}

export function visibleInputEntries(turn?: WorkflowTestTurn): Array<[string, string]> {
  return Object.entries(turnInputs(turn))
    .filter(([key]) => !key.startsWith('__'))
    .map(([key, value]): [string, string] => [
      key,
      typeof value === 'string' ? value : JSON.stringify(value),
    ])
    .filter(([, value]) => value !== undefined && value !== '');
}

export function parseLineList(value: string): string[] {
  return value
    .split(/\r?\n|,/)
    .map(item => item.trim())
    .filter(Boolean);
}

export function serializeLineList(values: string[] | undefined): string {
  return (values ?? []).join('\n');
}
