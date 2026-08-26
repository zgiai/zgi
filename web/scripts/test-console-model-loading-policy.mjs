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
const providerModelPageSource = fs.readFileSync(
  path.join(root, 'src/app/dashboard/provider/[providerId]/page.tsx'),
  'utf8'
);
const providerModelHookSource = modelHookSource.slice(
  modelHookSource.indexOf('export function useProviderModelsAll'),
  modelHookSource.indexOf('export function useAllModelsInfinite')
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
  providerModelHookSource,
  /isNetworkError\(requestError\)\s*&&\s*failureCount\s*<\s*1/,
  'provider model loading should retry one transient network error'
);
assert.match(
  providerModelHookSource,
  /id:\s*`provider-models-load-failed:\$\{provider \|\| 'unknown'\}`/,
  'provider model error toasts should be deduplicated per provider'
);
assert.match(
  modelHookSource,
  /id:\s*`available-models-load-failed:\$\{use_case\}`/,
  'available-model error toasts should be deduplicated per use case'
);
assert.match(
  providerModelPageSource,
  /error:\s*modelsError,[\s\S]*refetch:\s*refetchModels/,
  'the provider model page must consume the query error and refetch states'
);
assert.match(
  providerModelPageSource,
  /hasModelsLoadError\s*\?[\s\S]*aiProviders\.models\.loadErrorDescription[\s\S]*refetchModels\(\)/,
  'a failed provider model request must render a retryable error instead of an empty model state'
);
assert.match(
  providerModelPageSource,
  /hasModelsLoadError\s*=\s*Boolean\(modelsError\s*&&\s*allModels\.length\s*===\s*0\)/,
  'a background refresh failure must preserve already loaded provider models'
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
