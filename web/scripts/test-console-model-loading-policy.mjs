import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const consoleShellSource = fs.readFileSync(
  path.join(root, 'src/customer/default.tsx'),
  'utf8'
);
const modelHookSource = fs.readFileSync(
  path.join(root, 'src/hooks/model/use-model.ts'),
  'utf8'
);
const workChatSource = fs.readFileSync(
  path.join(root, 'src/app/console/work/chat/page.tsx'),
  'utf8'
);

assert.doesNotMatch(
  consoleShellSource,
  /ConsoleModelsPreloader|useAvailableModels/,
  'the console shell should not request models on every console page'
);
assert.match(
  modelHookSource,
  /isCanceledRequest\(error\)/,
  'canceled model requests should not show an error toast'
);
assert.match(
  modelHookSource,
  /isNetworkError\(requestError\)\s*&&\s*failureCount\s*<\s*1/,
  'available-model loading should retry one transient network error'
);
assert.match(
  modelHookSource,
  /id:\s*`available-models-load-failed:\$\{use_case\}`/,
  'available-model error toasts should be deduplicated per use case'
);
assert.match(
  workChatSource,
  /useCase:\s*'text-chat'/,
  'the general work chat should list text-chat models'
);
assert.doesNotMatch(
  workChatSource,
  /availabilityUseCase:\s*'agent-runtime'|modelAvailabilityUseCase="agent-runtime"/,
  'the general work chat must not hide text-chat models behind agent-runtime eligibility'
);

console.log('Console model loading policy checks passed.');
