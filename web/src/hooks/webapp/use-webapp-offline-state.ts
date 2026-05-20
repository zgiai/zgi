'use client';

import { useCallback, useEffect, useState } from 'react';
import { WEBAPP_OFFLINE_EVENT, emitWebAppOffline } from '@/utils/webapp/errors';

/**
 * @hook useWebAppOfflineState
 * @description Tracks the public WebApp offline signal for the current browser page.
 */
export function useWebAppOfflineState() {
  const [isOffline, setIsOffline] = useState(false);

  useEffect(() => {
    const handleOffline = () => setIsOffline(true);
    window.addEventListener(WEBAPP_OFFLINE_EVENT, handleOffline);
    return () => {
      window.removeEventListener(WEBAPP_OFFLINE_EVENT, handleOffline);
    };
  }, []);

  const markOffline = useCallback(() => {
    setIsOffline(true);
    emitWebAppOffline();
  }, []);

  return { isOffline, markOffline };
}
