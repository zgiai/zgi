import assert from 'node:assert/strict';
import { load, harness, jsx, nodes } from './connector-fixture.mjs';

let payload;
const h = harness(react => {
  const module = load('src/components/chat/file-upload-settings.tsx', {
    react,
    'react/jsx-runtime': jsx,
    '@/components/ui/switch': { Switch: 'Switch' },
    './file-upload-type-selector': { FileUploadTypeSelector: 'Types' },
    '@/i18n': { useT: () => key => key },
  });
  assert.equal(module.fileUploadSelectionEnabled(null), true);
  assert.equal(module.fileUploadSelectionEnabled({ allowed_file_types: [] }), false);
  assert.equal(module.fileUploadSelectionEnabled({ allowed_file_types: ['image'] }), true);
  return props => {
    const wrapper = module.PolicyFileUploadSettings({
      ...props,
      onChange: value => {
        payload = value;
      },
    });
    return { ...wrapper, rendered: module.FileUploadSettings(wrapper.props) };
  };
});
let tree = h.render({ value: { allowed_file_types: ['document'] } });
let toggle = nodes(tree.rendered).find(node => node.type === 'Switch');
assert.equal(toggle.props.checked, true);
assert.equal(toggle.props['aria-label'], 'files.title');
toggle.props.onCheckedChange(false);
assert.deepEqual(payload, { allowed_file_types: [] });
tree = h.render({ value: payload });
assert.equal(nodes(tree.rendered).filter(node => node.type === 'Types').length, 0);
toggle = nodes(tree.rendered).find(node => node.type === 'Switch');
assert.equal(toggle.props.checked, false);
toggle.props.onCheckedChange(true);
assert.deepEqual(
  payload,
  { allowed_file_types: ['document'] },
  'restore selected types after toggling'
);
tree = h.render({ value: payload });
nodes(tree.rendered)
  .find(node => node.type === 'Types')
  .props.onChange({ allowed_file_types: ['image'] });
tree = h.render({ value: payload, disabled: true });
assert.equal(nodes(tree.rendered).find(node => node.type === 'Switch').props.disabled, true);
assert.equal(nodes(tree.rendered).find(node => node.type === 'Types').props.disabled, true);
assert.deepEqual(tree.props.value, { allowed_file_types: ['image'] });
const { AgentRuntimeFileSection } = load(
  'src/components/agents/agent-runtime/sections/file-section.tsx',
  {
    'react/jsx-runtime': jsx,
    '@/components/chat/file-upload-settings': { FileUploadSettings: 'SharedSettings' },
    '@/i18n': { useT: () => key => key },
    '../runtime-section': { RuntimeSection: 'Section' },
  }
);
const agent = AgentRuntimeFileSection({ open: true, fileUploadEnabled: false, readOnly: true });
const shared = nodes(agent).find(node => node.type === 'SharedSettings');
assert.equal(shared.props.enabled, false);
assert.equal(shared.props.disabled, true);
console.log('Shared Agent/DigitalHuman upload settings checks passed');
