import type { AIChatRuntimeSurface } from '@/services/types/aichat';

/** The product surface that mounts the AI chat shell. */
export type AIChatShellSurface = 'aichat' | 'agent-draft' | 'agent-webapp' | 'digital-human';

export interface ChatSurfaceInput {
  surface: AIChatShellSurface;
  variant: 'full' | 'embedded';
  /** The runtime surface the host asked for; Agent and DigitalHuman pages override it. */
  runtimeSurface: AIChatRuntimeSurface;
  /** The host offers the sidebar's tool permission control. */
  enableToolGovernance: boolean;
  /** What the transport can do, read from the controller. */
  transport: {
    supportsToolGovernanceApproval: boolean;
    supportsRuntimeInput: boolean;
  };
}

/**
 * Everything the chat shell turns on or off for one surface (ADR 0050). The
 * shell reads this record instead of comparing surface names, so what a
 * surface offers is decided in one place and covered by one table of tests.
 */
export interface ChatSurfaceProfile {
  isEmbedded: boolean;
  /** The runtime surface sent with every request. */
  runtimeSurface: AIChatRuntimeSurface;
  background: 'bg-bg-canvas' | 'bg-background';
  /** The skill preference dialog and the connector picker. */
  skillAndConnectorPickers: boolean;
  /** Which wording the skill preference dialog uses. */
  skillPreferenceContext: 'platform-assistant' | 'conversation';
  toolGovernanceApprovals: boolean;
  /** The sidebar's permission tier control, and with it the asset audit. */
  toolGovernancePermissionControl: boolean;
  workflowApprovals: boolean;
  runtimeInput: boolean;
  /** Shows the memory key on messages. */
  memoryKey: boolean;
  /** Tool details and workflow failure details in the timeline. */
  executionDetails: boolean;
  /** The sidebar's page operation status in the timeline. */
  contextualOperationStatus: boolean;
  headerPosition: 'static' | 'overlay';
}

const PUBLISHED_PAGE_SURFACES: ReadonlySet<AIChatShellSurface> = new Set([
  'agent-draft',
  'agent-webapp',
  'digital-human',
]);

export function resolveChatSurfaceProfile(input: ChatSurfaceInput): ChatSurfaceProfile {
  const { surface, transport } = input;
  const isEmbedded = input.variant === 'embedded';
  // Agent and DigitalHuman chats run as external page chats whatever the host passes.
  const runtimeSurface: AIChatRuntimeSurface = PUBLISHED_PAGE_SURFACES.has(surface)
    ? 'external_page_chat'
    : input.runtimeSurface;
  const isSidebar = runtimeSurface === 'contextual_sidebar';
  const isAIChat = surface === 'aichat';
  return {
    isEmbedded,
    runtimeSurface,
    background:
      isEmbedded && surface !== 'agent-draft' && surface !== 'digital-human'
        ? 'bg-bg-canvas'
        : 'bg-background',
    skillAndConnectorPickers: isAIChat && (!isEmbedded || isSidebar),
    skillPreferenceContext: isSidebar ? 'platform-assistant' : 'conversation',
    toolGovernanceApprovals: transport.supportsToolGovernanceApproval,
    toolGovernancePermissionControl: isAIChat && isSidebar && input.enableToolGovernance,
    workflowApprovals: PUBLISHED_PAGE_SURFACES.has(surface),
    runtimeInput: transport.supportsRuntimeInput,
    memoryKey: surface !== 'agent-webapp' && surface !== 'digital-human',
    executionDetails: surface !== 'agent-webapp',
    contextualOperationStatus: isSidebar,
    headerPosition: surface === 'digital-human' ? 'static' : 'overlay',
  };
}
