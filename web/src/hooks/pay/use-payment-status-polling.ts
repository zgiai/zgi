import { useEffect, useRef, useCallback } from 'react';
import { payService } from '@/services/pay.service';
import type { OrderStatus } from '@/services/types/pay';

interface UsePaymentStatusPollingOptions {
  orderNo: string | null;
  enabled: boolean;
  interval?: number; // Polling interval in milliseconds, default 3000
  maxAttempts?: number; // Maximum polling attempts, default 200 (10 minutes at 3s interval)
  onSuccess?: (status: OrderStatus) => void;
  onFailure?: (status: OrderStatus) => void;
  onExpired?: () => void;
}

/**
 * Hook for polling payment status
 * Automatically stops polling when payment succeeds, fails, or reaches max attempts
 */
export function usePaymentStatusPolling({
  orderNo,
  enabled,
  interval = 3000,
  maxAttempts = 200,
  onSuccess,
  onFailure,
  onExpired,
}: UsePaymentStatusPollingOptions) {
  const attemptCountRef = useRef(0);
  const timerRef = useRef<NodeJS.Timeout | null>(null);
  const isPollingRef = useRef(false);
  // Store callbacks in refs to avoid dependency issues
  const callbacksRef = useRef({ onSuccess, onFailure, onExpired });

  // Update callbacks ref when they change
  useEffect(() => {
    callbacksRef.current = { onSuccess, onFailure, onExpired };
  }, [onSuccess, onFailure, onExpired]);

  const stopPolling = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    isPollingRef.current = false;
    attemptCountRef.current = 0;
  }, []);

  const checkPaymentStatus = useCallback(async () => {
    if (!orderNo || !enabled) {
      stopPolling();
      return;
    }

    attemptCountRef.current += 1;

    try {
      const response = await payService.getPaymentStatus(orderNo);
      const { status } = response.data;

      // Payment successful
      if (status === 'completed') {
        stopPolling();
        callbacksRef.current.onSuccess?.(status);
        return;
      }

      // Payment failed or cancelled
      if (status === 'failed' || status === 'cancelled') {
        stopPolling();
        callbacksRef.current.onFailure?.(status);
        return;
      }

      // Payment expired
      if (status === 'expired') {
        stopPolling();
        callbacksRef.current.onExpired?.();
        return;
      }

      // Continue polling if still pending and not exceeded max attempts
      if (attemptCountRef.current < maxAttempts) {
        timerRef.current = setTimeout(checkPaymentStatus, interval);
      } else {
        // Exceeded max attempts
        stopPolling();
        callbacksRef.current.onExpired?.();
      }
    } catch (error) {
      console.error('Failed to check payment status:', error);

      // Continue polling on error if not exceeded max attempts
      if (attemptCountRef.current < maxAttempts) {
        timerRef.current = setTimeout(checkPaymentStatus, interval);
      } else {
        stopPolling();
        callbacksRef.current.onExpired?.();
      }
    }
  }, [orderNo, enabled, interval, maxAttempts, stopPolling]);

  // Start polling when enabled and orderNo is available
  useEffect(() => {
    if (enabled && orderNo && !isPollingRef.current) {
      isPollingRef.current = true;
      attemptCountRef.current = 0;
      checkPaymentStatus();
    }

    // Cleanup on unmount or when dependencies change
    return () => {
      stopPolling();
    };
  }, [enabled, orderNo, checkPaymentStatus, stopPolling]);

  return {
    stopPolling,
    attemptCount: attemptCountRef.current,
  };
}
