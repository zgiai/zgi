// Setup module types
// English comments only. Strict TypeScript (no any).

export type SystemSetupStep = 'finished' | 'not_started';

export interface SystemSetupStatus {
  step: SystemSetupStep;
  // Present when initialized
  setup_at?: string;
}

export interface CreateSetupAdminRequest {
  email: string;
  name: string;
  password: string;
  language?: string;
}
