import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const chat = fs.readFileSync(
  path.join(root, 'src/components/chat/variants/aichat/aichat-chat.tsx'),
  'utf8'
);
const inputArea = fs.readFileSync(
  path.join(root, 'src/components/chat/variants/aichat/input-area.tsx'),
  'utf8'
);

assert.match(
  inputArea,
  /onModelPropsChange\?:\s*\(model:\s*ModelSelectorModelProps\s*\|\s*null\)\s*=>\s*void/,
  'the composer should expose the resolved selected-model capabilities to message actions'
);
assert.match(
  inputArea,
  /setSelectedModelProps\(nextModel\);[\s\S]*onModelPropsChange\?\.\(nextModel\)/,
  'the composer should keep local image-upload behavior and notify the parent together'
);
assert.match(
  chat,
  /selectedModelSupportsVision\s*===\s*false\s*&&\s*getReplayImageCount\(message\)\s*>\s*0[\s\S]*type:\s*'regenerate'/,
  'regeneration should be intercepted before it sends an image turn to a non-vision model'
);
assert.match(
  chat,
  /selectedModelSupportsVision\s*===\s*false\s*&&\s*getReplayImageCount\(message\)\s*>\s*0[\s\S]*type:\s*'edit'/,
  'edited historical turns should be intercepted before they send images to a non-vision model'
);
assert.match(
  chat,
  /<ConfirmDialog[\s\S]*visionReplayWarning\.title[\s\S]*visionReplayWarning\.description[\s\S]*visionReplayWarning\.recommendation[\s\S]*visionReplayWarning\.confirm/,
  'the warning should use a prominent confirmation dialog with an explicit consequence'
);

console.log('AIChat vision replay warning regression checks passed');
