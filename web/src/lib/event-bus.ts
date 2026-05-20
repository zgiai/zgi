/**
 * Event Bus - A communication bus for module interaction
 * Provides a publish-subscribe mode to decouple module dependencies
 */

type EventHandler<T = unknown> = (data: T) => void;

class EventBus {
  private events: Record<string, Array<EventHandler<unknown>>> = {};

  /**
   * Subscribe to an event
   * @param event Event name
   * @param handler Event handler function
   * @returns The function to unsubscribe
   */
  subscribe<T>(event: string, handler: EventHandler<T>) {
    if (!this.events[event]) {
      this.events[event] = [];
    }
    this.events[event].push(handler as EventHandler<unknown>);

    // Return the unsubscribe function
    return () => this.unsubscribe(event, handler);
  }

  /**
   * Unsubscribe from an event
   * @param event Event name
   * @param handler Event handler function
   */
  unsubscribe<T>(event: string, handler: EventHandler<T>) {
    if (!this.events[event]) {
      return;
    }
    this.events[event] = this.events[event].filter(h => h !== (handler as EventHandler<unknown>));
  }

  /**
   * Publish an event
   * @param event Event name
   * @param data Event data
   */
  publish<T>(event: string, data: T) {
    if (!this.events[event]) {
      return;
    }
    this.events[event].forEach(handler => handler(data));
  }

  /**
   * Clear all subscriptions for a specific event
   * @param event Event name
   */
  clear(event: string) {
    if (this.events[event]) {
      delete this.events[event];
    }
  }

  /**
   * Clear all event subscriptions
   */
  clearAll() {
    this.events = {};
  }
}

// Export singleton
export const eventBus = new EventBus();

export default eventBus;
