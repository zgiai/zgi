import { WEBAPP_LINK_SAME_TAB } from '@/lib/config';

/**
 * Opens an agent or workflow WebApp landing page from its draft. A new tab is
 * the default; NEXT_PUBLIC_WEBAPP_LINK_SAME_TAB=true opens it in this tab.
 */
export function openWebAppLandingPage(url: string): void {
  if (!url) return;
  if (WEBAPP_LINK_SAME_TAB) {
    window.location.href = url;
    return;
  }
  const opened = window.open('', '_blank');
  if (opened) {
    opened.opener = null;
    opened.location.href = url;
  } else {
    window.location.href = url;
  }
}
