import { readFileSync } from 'node:fs';

function assertIncludes(source, expected, message) {
  if (!source.includes(expected)) {
    throw new Error(message);
  }
}

const metadataEnvNames = [
  'NEXT_PUBLIC_APP_NAME',
  'NEXT_PUBLIC_APP_DESCRIPTION',
  'NEXT_PUBLIC_APP_KEYWORDS',
];

const dockerfile = readFileSync('Dockerfile', 'utf8');
const composeTemplate = readFileSync('../docker/docker-compose.template.yaml', 'utf8');
const standaloneCompose = readFileSync('docker-compose.yml', 'utf8');

for (const envName of metadataEnvNames) {
  assertIncludes(dockerfile, `ARG ${envName}`, `Dockerfile must declare ${envName} as a build arg.`);
  assertIncludes(
    dockerfile,
    `ENV ${envName}=\${${envName}}`,
    `Dockerfile must expose ${envName} to the Next.js build.`
  );

  for (const [name, source] of [
    ['Docker Compose template', composeTemplate],
    ['standalone Web Compose', standaloneCompose],
  ]) {
    assertIncludes(
      source,
      `${envName}: \${${envName}:-}`,
      `${name} must forward ${envName} to the Web image build.`
    );
  }
}

const nextConfig = readFileSync('next.config.mjs', 'utf8');
assertIncludes(nextConfig, 'APP_NAME_STATIC: staticAppName', 'App name must be frozen for metadata.');
assertIncludes(
  nextConfig,
  'APP_DESCRIPTION_STATIC: staticAppDescription',
  'App description must be frozen for metadata.'
);
assertIncludes(
  nextConfig,
  'APP_KEYWORDS_STATIC: staticAppKeywords',
  'App keywords must be frozen for metadata.'
);

const layout = readFileSync('src/app/layout.tsx', 'utf8');
assertIncludes(layout, 'title: APP_NAME', 'Root metadata must use the configured app name.');
assertIncludes(
  layout,
  'description: APP_DESCRIPTION',
  'Root metadata must include the configured description.'
);
assertIncludes(layout, 'keywords: APP_KEYWORDS', 'Root metadata must include configured keywords.');

console.log('Build metadata environment checks passed.');
