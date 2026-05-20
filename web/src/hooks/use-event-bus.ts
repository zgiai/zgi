'use client';

import { useEffect } from 'react';
import { eventBus } from '@/lib/event-bus';

/**
 * React hook for subscribing to EventBus events
 * Automatically handles subscription and cleanup on component unmount
 *
 * @param event Event name to subscribe to
 * @param handler Event handler function
 *
 * @example
 * // In a React component
 * useEventBus('order:created', (order) => {
 *   console.log('New order created:', order);
 *   // Update component state or trigger side effects
 * });
 */
export function useEventBus<T = unknown>(event: string, handler: (data: T) => void) {
  useEffect(() => {
    // Subscribe to the event when the component mounts
    const unsubscribe = eventBus.subscribe<T>(event, handler);

    // Unsubscribe when the component unmounts
    return unsubscribe;
  }, [event, handler]);
}

export default useEventBus;
