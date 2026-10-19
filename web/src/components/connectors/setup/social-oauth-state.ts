export function supportsSocialOAuth(connector: { id: string }): boolean {
  return ['x-social', 'linkedin-social', 'facebook-social'].includes(connector.id);
}

export function isOAuthFlowActive(flow: { status: string } | undefined): boolean {
  return Boolean(flow && ['pending', 'processing', 'select_page'].includes(flow.status));
}

/** Only navigate to TLS provider pages; never interpret returned URLs as script. */
export function safeOAuthAuthorizeURL(value: string | undefined): string {
  const url = new URL(value ?? '');
  if (url.protocol !== 'https:' || url.username || url.password) {
    throw new Error('Invalid OAuth authorization URL');
  }
  return url.href;
}
