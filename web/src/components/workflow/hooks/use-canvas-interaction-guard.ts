import React from 'react';

/**
 * Keeps short-lived canvas interactions from permanently hiding workflow panels
 * when browser/React Flow events are interrupted or not paired.
 */
export function useCanvasInteractionGuard() {
  const [isConnecting, setIsConnecting] = React.useState(false);
  const [isCanvasInteracting, setIsCanvasInteracting] = React.useState(false);
  const activeInteractionKeysRef = React.useRef<Set<string>>(new Set());
  const restoreInteractionTimerRef = React.useRef<number | null>(null);

  const clearRestoreInteractionTimer = React.useCallback(() => {
    if (restoreInteractionTimerRef.current === null) return;
    window.clearTimeout(restoreInteractionTimerRef.current);
    restoreInteractionTimerRef.current = null;
  }, []);

  const beginInteraction = React.useCallback(
    (key: string) => {
      clearRestoreInteractionTimer();
      activeInteractionKeysRef.current.add(key);
      setIsCanvasInteracting(true);
    },
    [clearRestoreInteractionTimer]
  );

  const finishInteraction = React.useCallback(
    (key: string, restoreDelay = 180) => {
      activeInteractionKeysRef.current.delete(key);
      clearRestoreInteractionTimer();

      if (activeInteractionKeysRef.current.size > 0) {
        setIsCanvasInteracting(true);
        return;
      }

      restoreInteractionTimerRef.current = window.setTimeout(() => {
        restoreInteractionTimerRef.current = null;
        if (activeInteractionKeysRef.current.size === 0) {
          setIsCanvasInteracting(false);
        }
      }, restoreDelay);
    },
    [clearRestoreInteractionTimer]
  );

  const finishAllInteractions = React.useCallback(
    (restoreDelay = 80) => {
      activeInteractionKeysRef.current.clear();
      clearRestoreInteractionTimer();

      restoreInteractionTimerRef.current = window.setTimeout(() => {
        restoreInteractionTimerRef.current = null;
        if (activeInteractionKeysRef.current.size === 0) {
          setIsConnecting(false);
          setIsCanvasInteracting(false);
        }
      }, restoreDelay);
    },
    [clearRestoreInteractionTimer]
  );

  const beginConnection = React.useCallback(() => {
    setIsConnecting(true);
    beginInteraction('connect');
  }, [beginInteraction]);

  const finishConnection = React.useCallback(() => {
    setIsConnecting(false);
    finishInteraction('connect');
  }, [finishInteraction]);

  React.useEffect(() => {
    return () => {
      clearRestoreInteractionTimer();
    };
  }, [clearRestoreInteractionTimer]);

  React.useEffect(() => {
    const finishAfterPointerRelease = () => {
      finishAllInteractions();
    };
    const finishImmediately = () => {
      finishAllInteractions(0);
    };
    const handleVisibilityChange = () => {
      if (document.visibilityState !== 'visible') finishImmediately();
    };

    window.addEventListener('pointerup', finishAfterPointerRelease, true);
    window.addEventListener('mouseup', finishAfterPointerRelease, true);
    window.addEventListener('touchend', finishAfterPointerRelease, true);
    window.addEventListener('touchcancel', finishImmediately, true);
    window.addEventListener('blur', finishImmediately);
    window.addEventListener('pagehide', finishImmediately);
    document.addEventListener('visibilitychange', handleVisibilityChange);

    return () => {
      window.removeEventListener('pointerup', finishAfterPointerRelease, true);
      window.removeEventListener('mouseup', finishAfterPointerRelease, true);
      window.removeEventListener('touchend', finishAfterPointerRelease, true);
      window.removeEventListener('touchcancel', finishImmediately, true);
      window.removeEventListener('blur', finishImmediately);
      window.removeEventListener('pagehide', finishImmediately);
      document.removeEventListener('visibilitychange', handleVisibilityChange);
    };
  }, [finishAllInteractions]);

  return {
    isConnecting,
    isCanvasInteracting,
    beginInteraction,
    finishInteraction,
    finishAllInteractions,
    beginConnection,
    finishConnection,
  };
}
