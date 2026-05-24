import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import Module from 'node:module';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
const ts = require('typescript');

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const srcRoot = path.join(webRoot, 'src');

const originalResolveFilename = Module._resolveFilename;
const originalLoad = Module._load;
const workflowNodesIndex = path.join(srcRoot, 'components/workflow/nodes/index.ts');

Module._resolveFilename = function resolveFilename(request, parent, isMain, options) {
  if (request.startsWith('@/')) {
    return originalResolveFilename.call(
      this,
      path.join(srcRoot, request.slice(2)),
      parent,
      isMain,
      options
    );
  }
  return originalResolveFilename.call(this, request, parent, isMain, options);
};

Module._extensions['.ts'] = (mod, filename) => {
  const source = require('node:fs').readFileSync(filename, 'utf8');
  const output = ts.transpileModule(source, {
    compilerOptions: {
      esModuleInterop: true,
      jsx: ts.JsxEmit.ReactJSX,
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
    },
    fileName: filename,
  }).outputText;
  mod._compile(output, filename);
};

Module._extensions['.tsx'] = Module._extensions['.ts'];

Module._load = function load(request, parent, isMain) {
  const resolved = Module._resolveFilename(request, parent, isMain);
  if (resolved === workflowNodesIndex) {
    return {
      NODE_TYPES: {
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
      },
    };
  }
  return originalLoad.call(this, request, parent, isMain);
};

const {
  validateWorkflow,
} = require('../src/components/workflow/store/helpers/validation-engine.ts');
const { AgentType } = require('../src/services/types/agent.ts');

const runnableSets = {
  mainRunnable: new Set(['tool-send-email', 'end']),
  iterRunnableMap: new Map(),
  commentSet: new Set(),
};

const nodes = [
  {
    id: 'start',
    type: 'custom',
    position: { x: 0, y: 0 },
    data: { type: 'start', title: 'Start', desc: '', variables: [] },
  },
  {
    id: 'tool-send-email',
    type: 'custom',
    position: { x: 240, y: 0 },
    data: {
      type: 'tools',
      title: '发送邮件',
      desc: '',
      provider_type: 'builtin',
      provider_id: 'email-provider',
      tool_name: 'send_email',
      tool_parameters: {
        recipient_email: { type: 'mixed', value: '' },
        subject: { type: 'mixed', value: '   ' },
        body: { type: 'mixed', value: '正文' },
      },
      isInLoop: false,
      isInIteration: false,
    },
  },
  {
    id: 'end',
    type: 'custom',
    position: { x: 480, y: 0 },
    data: { type: 'end', title: 'End', desc: '', outputs: [] },
  },
];

const edges = [
  {
    id: 'start-tool',
    source: 'start',
    target: 'tool-send-email',
    sourceHandle: 'source',
    targetHandle: 'target',
    data: { sourceType: 'start', targetType: 'tools', isInLoop: false },
  },
  {
    id: 'tool-end',
    source: 'tool-send-email',
    target: 'end',
    sourceHandle: 'source',
    targetHandle: 'target',
    data: { sourceType: 'tools', targetType: 'end', isInLoop: false },
  },
];

const toolProviders = [
  {
    id: 'email-provider',
    name: 'email-provider',
    plugin_id: 'plugin-email',
    plugin_unique_identifier: 'plugin-email',
    tools: [
      {
        name: 'send_email',
        parameters: [
          {
            name: 'recipient_email',
            type: 'string',
            required: true,
            label: { zh_Hans: '收件人邮箱', en_US: 'Recipient email' },
          },
          {
            name: 'subject',
            type: 'string',
            required: true,
            label: { zh_Hans: '邮件主题', en_US: 'Subject' },
          },
          {
            name: 'body',
            type: 'string',
            required: true,
            label: { zh_Hans: '邮件内容', en_US: 'Body' },
          },
        ],
      },
    ],
  },
];

const result = validateWorkflow(
  nodes,
  edges,
  AgentType.WORKFLOW,
  runnableSets,
  null,
  toolProviders,
  'zh-Hans'
);

const toolErrors = result.errors.filter(error => error.nodeId === 'tool-send-email');

assert.deepEqual(
  toolErrors.map(error => [error.code, error.params?.name]),
  [
    ['tool.validation.paramValueRequired', '收件人邮箱'],
    ['tool.validation.paramValueRequired', '邮件主题'],
  ]
);
assert.equal(result.errorMap.get('tool-send-email')?.length, 2);
assert.equal(
  result.errors.some(error => error.code === 'workflow.validation.startExactlyOne'),
  false
);

const validNodes = JSON.parse(JSON.stringify(nodes));
validNodes[1].data.tool_parameters.recipient_email.value = 'user@example.com';
validNodes[1].data.tool_parameters.subject.value = '主题';

const validResult = validateWorkflow(
  validNodes,
  edges,
  AgentType.WORKFLOW,
  runnableSets,
  null,
  toolProviders,
  'zh-Hans'
);

assert.equal(validResult.errors.filter(error => error.nodeId === 'tool-send-email').length, 0);

console.log('workflow tool validation tests passed');
