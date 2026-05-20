let logoutInProgress = false;
let authRedirectInProgress = false;
let authRedirectTimer: ReturnType<typeof setTimeout> | null = null;

export function setLogoutInProgress(value: boolean): void {
  logoutInProgress = value;
}

export function isLogoutInProgress(): boolean {
  return logoutInProgress;
}

export function markAuthRedirectInProgress(): void {
  authRedirectInProgress = true;

  if (authRedirectTimer) {
    clearTimeout(authRedirectTimer);
  }

  authRedirectTimer = setTimeout(() => {
    authRedirectInProgress = false;
    authRedirectTimer = null;
  }, 10_000);
}

export function isAuthRedirectInProgress(): boolean {
  return authRedirectInProgress;
}
