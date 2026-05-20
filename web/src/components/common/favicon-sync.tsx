'use client';

import { useEffect } from 'react';
import { FAVICON_URL } from '@/lib/config';

interface FaviconSyncProps {
  faviconUrl?: string;
}

/**
 * @component FaviconSync
 * @category Common
 * @status Stable
 * @description Synchronizes favicon links from runtime public env overrides on the client
 * @usage Mount once near the app root to keep favicon links aligned with runtime env configuration
 * @example
 * <FaviconSync />
 */
export function FaviconSync({ faviconUrl }: FaviconSyncProps) {
  useEffect(() => {
    const resolvedFaviconUrl = faviconUrl || FAVICON_URL;
    if (!resolvedFaviconUrl) return;

    const links: Array<{ rel: string; id: string }> = [
      { rel: 'icon', id: 'app-favicon-icon' },
      { rel: 'shortcut icon', id: 'app-favicon-shortcut' },
      { rel: 'apple-touch-icon', id: 'app-favicon-apple' },
    ];

    for (const { rel, id } of links) {
      let link = document.head.querySelector<HTMLLinkElement>(`#${id}`);

      if (!link) {
        link = document.createElement('link');
        link.id = id;
        link.rel = rel;
        document.head.appendChild(link);
      }

      link.href = resolvedFaviconUrl;
    }
  }, [faviconUrl]);

  return null;
}
