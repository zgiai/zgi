import assert from 'node:assert/strict';
import { jsx, load, nodes } from './connector-fixture.mjs';

for (const locale of ['zh-Hans', 'en-US']) {
  const messages = load(`src/i18n/modules/agents/${locale}.ts`).default.agentRuntime;
  const { AgentRuntimeExecutionSection } = load(
    'src/components/agents/agent-runtime/sections/execution-section.tsx',
    {
      'react/jsx-runtime': jsx,
      '@/components/ui/switch': { Switch: 'Switch' },
      '@/i18n': {
        useT: () => key => key.split('.').reduce((value, part) => value[part], messages),
      },
      '../runtime-section': { RuntimeSection: 'RuntimeSection' },
    }
  );

  assert.deepEqual(Object.keys(messages.execution).sort(), ['description', 'title']);
  assert.doesNotMatch(messages.execution.description, /仅控制台|console.*only|Profile|30/);
  for (const value of [null, { enabled: false }, { enabled: true }]) {
    for (const readOnly of [false, true]) {
      let changed;
      const tree = AgentRuntimeExecutionSection({
        open: true,
        value,
        readOnly,
        onToggleSection: () => {},
        onChange: next => {
          changed = next;
        },
      });
      const rendered = nodes(tree);
      const descriptions = rendered.filter(node => node.type === 'p');
      assert.equal(descriptions.length, 1, 'show only one concise description');
      assert.equal(descriptions[0].props.children, messages.execution.description);
      const toggle = rendered.find(node => node.type === 'Switch');
      assert.equal(toggle.props.checked, value?.enabled ?? false);
      assert.equal(toggle.props.disabled, readOnly);
      assert.equal(toggle.props['aria-label'], messages.execution.title);
      toggle.props.onCheckedChange(!toggle.props.checked);
      assert.deepEqual(changed, { enabled: !toggle.props.checked });
    }
  }
}

console.log('Agent execution section checks passed');
