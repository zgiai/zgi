import type { ReactNode } from 'react';

export type OperationCardTone = 'neutral' | 'info' | 'success' | 'warning' | 'destructive';

export type OperationCardActionVariant =
  | 'default'
  | 'secondary'
  | 'outline'
  | 'ghost'
  | 'destructive';

export interface OperationCardAction {
  id: string;
  label: string;
  title?: string;
  icon?: ReactNode;
  variant?: OperationCardActionVariant;
  disabled?: boolean;
  loading?: boolean;
  href?: string;
  external?: boolean;
  onClick?: () => void;
}

export interface OperationCardMetaItem {
  id: string;
  label: string;
  value?: ReactNode;
  description?: ReactNode;
  icon?: ReactNode;
  tone?: OperationCardTone;
}

export interface OperationCardTextBlock {
  id: string;
  title?: ReactNode;
  description?: ReactNode;
  tone?: OperationCardTone;
  icon?: ReactNode;
}

export type OperationPlanStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';

export type OperationPlanStepStatus = 'pending' | 'running' | 'completed' | 'failed' | 'skipped';

export interface OperationPlanStep {
  id: string;
  title: ReactNode;
  description?: ReactNode;
  status?: OperationPlanStepStatus;
  statusLabel?: string;
  meta?: OperationCardMetaItem[];
}

export interface OperationPlanCardProps {
  title?: ReactNode;
  description?: ReactNode;
  status?: OperationPlanStatus;
  statusLabel?: string;
  eyebrow?: ReactNode;
  steps?: OperationPlanStep[];
  meta?: OperationCardMetaItem[];
  actions?: OperationCardAction[];
  compact?: boolean;
  className?: string;
}

export type OperationConfirmationStatus = 'pending' | 'confirmed' | 'rejected' | 'expired';

export interface OperationConfirmationCardProps {
  title?: ReactNode;
  description?: ReactNode;
  status?: OperationConfirmationStatus;
  statusLabel?: string;
  eyebrow?: ReactNode;
  summary?: ReactNode;
  items?: OperationCardMetaItem[];
  warnings?: OperationCardTextBlock[];
  confirmAction?: OperationCardAction;
  cancelAction?: OperationCardAction;
  actions?: OperationCardAction[];
  compact?: boolean;
  className?: string;
}

export type OperationResultStatus = 'success' | 'error' | 'warning' | 'info';

export interface OperationResultArtifact {
  id: string;
  label: string;
  href?: string;
  description?: ReactNode;
  icon?: ReactNode;
  external?: boolean;
  onClick?: () => void;
}

export interface OperationResultCardProps {
  title?: ReactNode;
  description?: ReactNode;
  status?: OperationResultStatus;
  statusLabel?: string;
  eyebrow?: ReactNode;
  metrics?: OperationCardMetaItem[];
  artifacts?: OperationResultArtifact[];
  details?: OperationCardTextBlock[];
  actions?: OperationCardAction[];
  compact?: boolean;
  className?: string;
}

export type HostedTaskStatus =
  | 'queued'
  | 'running'
  | 'paused'
  | 'completed'
  | 'failed'
  | 'cancelled';

export interface HostedTaskCardProps {
  title?: ReactNode;
  description?: ReactNode;
  status?: HostedTaskStatus;
  statusLabel?: string;
  eyebrow?: ReactNode;
  progress?: number;
  progressLabel?: ReactNode;
  currentStep?: ReactNode;
  meta?: OperationCardMetaItem[];
  actions?: OperationCardAction[];
  compact?: boolean;
  className?: string;
}
