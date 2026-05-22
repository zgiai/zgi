const WORKFLOW_VALUE_TOKEN_PATTERN = /^\{\{#[^#]+#\}\}$/;

export function isWorkflowValueToken(value: string): boolean {
  return WORKFLOW_VALUE_TOKEN_PATTERN.test(value.trim());
}
