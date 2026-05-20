const LINK_CODE_PATTERN = /^[A-Za-z0-9]+$/;
const WORKFLOW_VALUE_TOKEN_PATTERN = /^\{\{#[^#]+#\}\}$/;

interface LinkCodeValidationOptions {
  allowWorkflowToken?: boolean;
}

export function isWorkflowValueToken(value: string): boolean {
  return WORKFLOW_VALUE_TOKEN_PATTERN.test(value.trim());
}

export function isNotificationSMSLinkCodeValid(
  value: string,
  options: LinkCodeValidationOptions = {}
): boolean {
  const trimmed = value.trim();

  if (!trimmed) {
    return false;
  }

  if (LINK_CODE_PATTERN.test(trimmed)) {
    return true;
  }

  return Boolean(options.allowWorkflowToken && isWorkflowValueToken(trimmed));
}
