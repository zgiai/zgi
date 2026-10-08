import assert from 'node:assert/strict';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import * as jsxRuntime from 'react/jsx-runtime';
import ts from 'typescript';
import { load, read } from './connector-fixture.mjs';

const chat = read('src/components/chat/variants/aichat/aichat-chat.tsx');
assert.match(read('src/components/chat/index.tsx'), /showSkillEventDetails\?: boolean/);
assert.match(chat, /showSkillEventDetails=\{surface !== 'agent-webapp' && showSkillEventDetails\}/);
assert.match(chat, /showSkillEventDetails = true/, 'internal chat keeps diagnostic details');
assert.match(
  read('src/components/digital-human/digital-human-webapp.tsx'),
  /showSkillEventDetails=\{false\}/,
  'published DigitalHuman apps also hide tool details'
);
assert.doesNotMatch(
  read('src/components/digital-human/digital-human-chat.tsx'),
  /showSkillEventDetails=\{false\}/,
  'DigitalHuman console debugging keeps its details'
);
for (const file of ['message-list.tsx', 'message-bubble.tsx']) {
  assert.match(
    read(`src/components/chat/variants/aichat/${file}`),
    /showSkillEventDetails=\{showSkillEventDetails\}/,
    'both live and historical messages must receive the visibility policy'
  );
}

const { ToolInvocationRow } = load('src/components/chat/variants/aichat/tool-invocation-row.tsx', {
  react: React,
  'react/jsx-runtime': jsxRuntime,
  'lucide-react': { ChevronDown: () => null },
  '@/lib/utils': { cn: (...values) => values.filter(value => typeof value === 'string').join(' ') },
});
// Execute the actual timeline row with narrow presentation fixtures, avoiding
// unrelated governance/controller hooks while testing what reaches the DOM.
const source = read('src/components/chat/variants/aichat/agentic-timeline.tsx');
const ast = ts.createSourceFile(
  'timeline.tsx',
  source,
  ts.ScriptTarget.Latest,
  true,
  ts.ScriptKind.TSX
);
const row = ast.statements.find(
  statement => ts.isFunctionDeclaration(statement) && statement.name?.text === 'SkillTimelineRow'
);
assert.ok(row, 'real SkillTimelineRow must exist');
const compiled = ts.transpileModule(row.getText(ast), {
  compilerOptions: {
    module: ts.ModuleKind.CommonJS,
    target: ts.ScriptTarget.ES2022,
    jsx: ts.JsxEmit.ReactJSX,
  },
}).outputText;
const fixtures = {
  useT: () => key => key,
  useLocale: () => ({ locale: 'en-US' }),
  isConnectorOperationInvocation: () => false,
  parseConnectorOperationPresentation: () => null,
  getAgentExecutionJobPresentation: () => ({ labelKey: 'job-status', exitCode: 0 }),
  getAgentExecutionDisplay: () => ({ hasOutput: true }),
  getDurationText: () => '140ms',
  agentExecutionDuration: () => 140,
  sanitizeDisplayString: value => value,
  sanitizeTimelineResultForDisplay: value => value,
  ToolInvocationRow,
  Terminal: () => null,
  AIChatSkillIcon: () => null,
  AIChatSkillResultSummary: () => React.createElement('p', null, 'private-result'),
  getStatusIcon: () => null,
  timelineDebugRows: () => [['arguments', 'private-command']],
  formatAIChatTimelineArgumentSummary: value => value,
  formatDebugValue: value => value,
  TIMELINE_DEBUG_LABEL_KEYS: { arguments: 'arguments' },
};
const SkillTimelineRow = new Function(
  'require',
  'exports',
  ...Object.keys(fixtures),
  `${compiled}\nreturn SkillTimelineRow;`
)(
  name => {
    assert.equal(name, 'react/jsx-runtime');
    return jsxRuntime;
  },
  {},
  ...Object.values(fixtures)
);
for (const toolName of ['shell_exec', 'workspace_export_file', 'run_script']) {
  for (const tone of ['running', 'success', 'error']) {
    const event = {
      tone,
      title: `safe-${toolName}-${tone}`,
      detail: 'private-detail',
      skill: {},
      item: {
        invocation: {
          skill_id: toolName === 'run_script' ? 'legacy-skill' : 'agent_execution',
          tool_name: toolName,
          result: {},
        },
      },
    };
    const render = showDetails =>
      renderToStaticMarkup(React.createElement(SkillTimelineRow, { event, showDetails }));
    const publicHTML = render(false);
    assert.ok(publicHTML.includes(event.title), 'WebApp keeps safe execution status');
    assert.ok(publicHTML.includes('140ms'), 'WebApp keeps duration');
    assert.doesNotMatch(
      publicHTML,
      /private-|technicalDetails|<button|<details|aria-expanded/,
      'WebApp must not mount detail content or an expandable tool row'
    );
    const consoleHTML = render(true);
    assert.match(consoleHTML, /technicalDetails/);
    assert.match(consoleHTML, /private-command/);
    assert.match(consoleHTML, /aria-expanded/);
  }
}
console.log('WebApp status-only tool events and console detail rendering passed');
