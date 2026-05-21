const LINK_SUFFIX_PATTERN = /^[A-Za-z0-9/_?=&.%+-]+$/;
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

  if (/^(https?:)?\/\//i.test(trimmed) || /\s/.test(trimmed)) {
    return false;
  }

  if (options.allowWorkflowToken && isWorkflowValueToken(trimmed)) {
    return true;
  }

  return LINK_SUFFIX_PATTERN.test(trimmed);
}
