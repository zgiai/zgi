import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const files = {
  workChat: fs.readFileSync(path.join(root, 'src/app/console/work/chat/page.tsx'), 'utf8'),
  chatEntry: fs.readFileSync(path.join(root, 'src/components/chat/index.tsx'), 'utf8'),
  chatShell: fs.readFileSync(
    path.join(root, 'src/components/chat/variants/aichat/aichat-chat.tsx'),
    'utf8'
  ),
  inputArea: fs.readFileSync(
    path.join(root, 'src/components/chat/variants/aichat/input-area.tsx'),
    'utf8'
  ),
  inputToolbar: fs.readFileSync(
    path.join(root, 'src/components/chat/variants/aichat/input-toolbar.tsx'),
    'utf8'
  ),
  modelSelector: fs.readFileSync(
    path.join(root, 'src/components/common/model-selector/model-selector/index.tsx'),
    'utf8'
  ),
};

assert.match(
  files.workChat,
  /modelSelectorWarning=\{modelPrecheckWarnings\.length > 0\}/,
  'work chat should mark the selector only when the model-level precheck exposes warnings'
);
assert.match(
  files.chatEntry,
  /modelSelectorWarning\?: boolean/,
  'the chat entry contract should carry the selected model warning state'
);
assert.match(
  files.chatShell,
  /modelSelectorWarning=\{modelSelectorWarning\}/,
  'the chat shell should pass the warning state to the composer'
);
assert.match(
  files.inputArea,
  /modelSelectorWarning=\{modelSelectorWarning\}/,
  'the composer should pass the warning state to the toolbar'
);
assert.match(
  files.inputToolbar,
  /selectedModelWarning=\{[\s\S]*consoleChat\.modelPrecheck\.title[\s\S]*\}/,
  'the toolbar should give the model selector an accessible warning label'
);
assert.match(
  files.modelSelector,
  /selectedModelWarning\?: string/,
  'the model selector should expose a warning marker contract for the selected model'
);
assert.match(
  files.modelSelector,
  /data-model-warning=\{selectedModelWarning \? 'true' : undefined\}/,
  'the model selector trigger should expose its warning state for accessibility and browser tests'
);
assert.match(
  files.modelSelector,
  /<AlertTriangle[\s\S]*aria-hidden="true"/,
  'the selector should render a visual warning icon without duplicating the accessible label'
);
assert.match(
  files.modelSelector,
  /className="sr-only"/,
  'the selector warning should remain available to screen readers'
);

const triggerClassNames = files.modelSelector.match(
  /<SelectTrigger[\s\S]*?className=\{cn\(([\s\S]*?)\)\}\s*id=/
)?.[1];
assert.ok(triggerClassNames, 'the model selector trigger should compose semantic class names');
assert.ok(
  triggerClassNames.indexOf('className') < triggerClassNames.indexOf('selectedModelWarning'),
  'the warning border and background should override neutral classes supplied by callers'
);

console.log('model selector risk marker regression checks passed');
