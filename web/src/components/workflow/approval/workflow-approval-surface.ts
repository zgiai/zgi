import type { ApprovalRuntimeForm } from '@/services/approval.service';

export type WorkflowApprovalSurface =
  | 'aichat'
  | 'agent-draft'
  | 'agent-webapp'
  | 'workflow-debug'
  | 'workflow-webapp';

interface WorkflowApprovalInlineAccess {
  surface: WorkflowApprovalSurface;
  form?: ApprovalRuntimeForm | null;
  uiApprovalAllowed?: boolean;
}

/**
 * Keeps approval-channel policy outside presentation components. Draft and
 * debugger surfaces are trusted operator surfaces; published surfaces must
 * have an explicit server decision or an enabled WebApp submit method.
 */
export function isWorkflowApprovalInlineAllowed({
  surface,
  form,
  uiApprovalAllowed,
}: WorkflowApprovalInlineAccess): boolean {
  if (surface === 'agent-draft' || surface === 'workflow-debug') return true;
  if (typeof uiApprovalAllowed === 'boolean') return uiApprovalAllowed;

  const webappMethod = form?.submit_methods?.webapp;
  return webappMethod ? webappMethod.enabled !== false : false;
}
