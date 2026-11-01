import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { URL } from 'node:url';
import ts from 'typescript';

// ADR 0050 step 2a: what each chat surface offers is decided by one profile.

const source = await readFile(
  new URL('../src/components/chat/variants/aichat/chat-surface-profile.ts', import.meta.url),
  'utf8'
);
const { resolveChatSurfaceProfile } = await import(
  `data:text/javascript;base64,${Buffer.from(
    ts.transpileModule(source, {
      compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
    }).outputText
  ).toString('base64')}`
);

const full = { supportsToolGovernanceApproval: true, supportsRuntimeInput: true };
const none = { supportsToolGovernanceApproval: false, supportsRuntimeInput: false };

// The surfaces as their pages mount them today.
const rows = [
  [
    'console sidebar assistant',
    {
      surface: 'aichat',
      variant: 'embedded',
      runtimeSurface: 'contextual_sidebar',
      enableToolGovernance: true,
      transport: full,
    },
    {
      runtimeSurface: 'contextual_sidebar',
      background: 'bg-bg-canvas',
      skillAndConnectorPickers: true,
      skillPreferenceContext: 'platform-assistant',
      toolGovernanceApprovals: true,
      toolGovernancePermissionControl: true,
      workflowApprovals: false,
      memoryKey: true,
      executionDetails: true,
      contextualOperationStatus: true,
      headerPosition: 'overlay',
    },
  ],
  [
    'console chat page',
    {
      surface: 'aichat',
      variant: 'full',
      runtimeSurface: 'work_chat',
      enableToolGovernance: false,
      transport: full,
    },
    {
      runtimeSurface: 'work_chat',
      background: 'bg-background',
      skillAndConnectorPickers: true,
      skillPreferenceContext: 'conversation',
      toolGovernancePermissionControl: false,
      workflowApprovals: false,
      contextualOperationStatus: false,
    },
  ],
  [
    'AI chat embedded outside the sidebar',
    {
      surface: 'aichat',
      variant: 'embedded',
      runtimeSurface: 'work_chat',
      enableToolGovernance: false,
      transport: full,
    },
    { skillAndConnectorPickers: false, background: 'bg-bg-canvas' },
  ],
  [
    'Agent preview',
    {
      surface: 'agent-draft',
      variant: 'embedded',
      runtimeSurface: 'work_chat',
      enableToolGovernance: false,
      transport: full,
    },
    {
      runtimeSurface: 'external_page_chat',
      background: 'bg-background',
      skillAndConnectorPickers: false,
      toolGovernanceApprovals: true,
      workflowApprovals: true,
      memoryKey: true,
      executionDetails: true,
    },
  ],
  [
    'published Agent webapp',
    {
      surface: 'agent-webapp',
      variant: 'full',
      runtimeSurface: 'work_chat',
      enableToolGovernance: false,
      transport: full,
    },
    {
      runtimeSurface: 'external_page_chat',
      toolGovernanceApprovals: true,
      workflowApprovals: true,
      memoryKey: false,
      executionDetails: false,
      headerPosition: 'overlay',
    },
  ],
  [
    'DigitalHuman chat',
    {
      surface: 'digital-human',
      variant: 'full',
      runtimeSurface: 'work_chat',
      enableToolGovernance: false,
      transport: full,
    },
    {
      runtimeSurface: 'external_page_chat',
      background: 'bg-background',
      toolGovernanceApprovals: true,
      workflowApprovals: true,
      memoryKey: false,
      executionDetails: true,
      headerPosition: 'static',
    },
  ],
  [
    'DigitalHuman preview, whose transport cannot continue approvals',
    {
      surface: 'digital-human',
      variant: 'embedded',
      runtimeSurface: 'work_chat',
      enableToolGovernance: false,
      transport: none,
    },
    { toolGovernanceApprovals: false, runtimeInput: false, background: 'bg-background' },
  ],
];
for (const [name, input, expected] of rows) {
  const profile = resolveChatSurfaceProfile(input);
  for (const [key, value] of Object.entries(expected)) {
    assert.equal(profile[key], value, `${name}: ${key}`);
  }
}

// Rules that hold for every combination.
const surfaces = ['aichat', 'agent-draft', 'agent-webapp', 'digital-human'];
for (const surface of surfaces) {
  for (const variant of ['full', 'embedded']) {
    for (const runtimeSurface of ['work_chat', 'contextual_sidebar', 'external_page_chat']) {
      for (const enableToolGovernance of [false, true]) {
        for (const transport of [full, none]) {
          const input = { surface, variant, runtimeSurface, enableToolGovernance, transport };
          const profile = resolveChatSurfaceProfile(input);
          const label = JSON.stringify(input);
          assert.equal(profile.isEmbedded, variant === 'embedded', label);
          assert.equal(
            profile.toolGovernanceApprovals,
            transport.supportsToolGovernanceApproval,
            label
          );
          assert.equal(profile.runtimeInput, transport.supportsRuntimeInput, label);
          if (surface !== 'aichat') {
            assert.equal(profile.runtimeSurface, 'external_page_chat', label);
            assert.equal(profile.skillAndConnectorPickers, false, label);
            assert.equal(profile.toolGovernancePermissionControl, false, label);
            assert.equal(profile.workflowApprovals, true, label);
          } else {
            assert.equal(profile.runtimeSurface, runtimeSurface, label);
            assert.equal(profile.workflowApprovals, false, label);
            assert.equal(
              profile.toolGovernancePermissionControl,
              runtimeSurface === 'contextual_sidebar' && enableToolGovernance,
              label
            );
          }
          if (profile.toolGovernancePermissionControl) {
            assert.equal(profile.contextualOperationStatus, true, label);
          }
        }
      }
    }
  }
}

console.log(`chat surface profile: ${rows.length} surfaces and every combination ok`);
