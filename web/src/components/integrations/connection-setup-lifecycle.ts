export interface ConnectionSetupInitialization {
  initializedConnectionID: string | null;
  shouldReset: boolean;
}

export const CURRENT_CONNECTION_SETUP_VERSION = 2;

export const CONNECTION_SETUP_TEST_REUSE_WINDOW_MS = 15_000;

export type AutomaticConnectionVerificationDecision = 'run' | 'reuse' | 'skip';

export function resolveAutomaticConnectionVerification({
  open,
  initialStep,
  verified,
  lastTestedAt,
  now = Date.now(),
  reuseWindowMs = CONNECTION_SETUP_TEST_REUSE_WINDOW_MS,
}: {
  open: boolean;
  initialStep: number;
  verified: boolean;
  lastTestedAt?: string | null;
  now?: number;
  reuseWindowMs?: number;
}): AutomaticConnectionVerificationDecision {
  if (!open || initialStep !== 0) return 'skip';
  if (!verified || !lastTestedAt) return 'run';

  const testedAt = Date.parse(lastTestedAt);
  if (!Number.isFinite(testedAt)) return 'run';

  return now - testedAt >= 0 && now - testedAt <= reuseWindowMs ? 'reuse' : 'run';
}

export function connectionNeedsSetup(connection: {
  setup_version?: number;
  setup_completed_at?: string | null;
}): boolean {
  return (
    !connection.setup_completed_at ||
    (connection.setup_version ?? 1) < CURRENT_CONNECTION_SETUP_VERSION
  );
}

export function resolveConnectionSetupInitialization(
  initializedConnectionID: string | null,
  connectionID: string | null,
  open: boolean
): ConnectionSetupInitialization {
  if (!open) {
    return { initializedConnectionID: null, shouldReset: false };
  }

  return {
    initializedConnectionID: connectionID,
    shouldReset: initializedConnectionID !== connectionID,
  };
}
