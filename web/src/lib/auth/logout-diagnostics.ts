import { sessionManager } from './session-manager';
import { isLogoutInProgress } from './logout-state';
import { describeAccessLoadError } from '@/utils/access-load-error';

type LogoutPhase =
  | 'started'
  | 'request_completed'
  | 'request_failed'
  | 'session_cleared'
  | 'client_state_cleared'
  | 'cleanup_failed'
  | 'redirecting';

/** Never include session values, URLs, account IDs or raw transport errors. */
export function reportLogoutPhase(phase: LogoutPhase, error?: unknown): void {
  try {
    console.info('auth.logout.phase', {
      phase,
      session_present: sessionManager.hasSession(),
      logout_in_progress: isLogoutInProgress(),
      ...(error === undefined ? {} : describeAccessLoadError(error)),
    });
  } catch {
    // Diagnostics must never interrupt logout or replace its original error.
  }
}
