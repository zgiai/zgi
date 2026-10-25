export function supportsSocialOAuth(connector: { id: string }): boolean {
  return ['x-social', 'linkedin-social', 'facebook-social', 'github'].includes(connector.id);
}

export function isOAuthFlowActive(flow: { status: string } | undefined): boolean {
  return Boolean(
    flow && ['pending', 'processing', 'select_page', 'install_app'].includes(flow.status)
  );
}

/** Only navigate to TLS provider pages; never interpret returned URLs as script. */
export function safeOAuthAuthorizeURL(value: string | undefined): string {
  const url = new URL(value ?? '');
  if (url.protocol !== 'https:' || url.username || url.password) {
    throw new Error('Invalid OAuth authorization URL');
  }
  return url.href;
}

/** Installation links are restricted to the GitHub App installation page. */
export function safeGitHubInstallURL(value: string | undefined): string | undefined {
  try {
    const url = new URL(safeOAuthAuthorizeURL(value));
    if (
      url.origin !== 'https://github.com' ||
      !/^\/apps\/[a-z0-9-]+\/installations\/new$/.test(url.pathname)
    ) {
      return undefined;
    }
    return url.href;
  } catch {
    return undefined;
  }
}
