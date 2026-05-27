import { spawn } from 'node:child_process';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);

const BUILD_RESOURCE_DEFAULTS = {
  NEXT_BUILD_CPUS: '2',
  NEXT_STATIC_GENERATION_MAX_CONCURRENCY: '2',
  NEXT_TURBOPACK_MEMORY_LIMIT_MB: '2048',
  NEXT_BUILD_NODE_MEMORY_MB: '2048',
  NEXT_WEBPACK_MEMORY_OPTIMIZATIONS: 'true',
};

function withBuildResourceDefaults() {
  const env = { ...process.env };

  for (const [name, value] of Object.entries(BUILD_RESOURCE_DEFAULTS)) {
    if (!env[name]) {
      env[name] = value;
    }
  }

  const memoryMb = env.NEXT_BUILD_NODE_MEMORY_MB;
  if (memoryMb && !/\b--max-old-space-size=/.test(env.NODE_OPTIONS ?? '')) {
    env.NODE_OPTIONS = [env.NODE_OPTIONS, `--max-old-space-size=${memoryMb}`]
      .filter(Boolean)
      .join(' ');
  }

  return env;
}

function runNodeScript(scriptPath, env) {
  return run(process.execPath, [scriptPath], env);
}

function run(command, args, env) {
  return new Promise((resolveRun, rejectRun) => {
    const child = spawn(command, args, {
      cwd: process.cwd(),
      env,
      shell: false,
      stdio: 'inherit',
    });

    child.on('error', rejectRun);
    child.on('close', code => {
      if (code === 0) {
        resolveRun();
        return;
      }
      rejectRun(new Error(`${command} ${args.join(' ')} exited with code ${code}`));
    });
  });
}

const nextBuildArgs = process.argv.slice(2);
if (nextBuildArgs[0] === '--') {
  nextBuildArgs.shift();
}
const env = withBuildResourceDefaults();

await runNodeScript('scripts/generate-sensitive-word-filter.mjs', env);
await runNodeScript('scripts/check-i18n-route-modules.mjs', env);
await run(process.execPath, [require.resolve('next/dist/bin/next'), 'build', ...nextBuildArgs], env);
