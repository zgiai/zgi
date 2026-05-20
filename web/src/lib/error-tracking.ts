/**
 * Global error tracking service
 * Centralized error handling and reporting
 */

import { NODE_ENV, APP_VERSION } from '@/lib/config';

interface ErrorMetadata {
  userId?: string;
  url?: string;
  componentStack?: string;
  tags?: Record<string, string>;
  [key: string]: unknown;
}

type ErrorHandler = (error: Error, metadata?: ErrorMetadata) => void;

class ErrorTrackingService {
  private static instance: ErrorTrackingService;
  private isInitialized = false;
  private handlers: ErrorHandler[] = [];
  private defaultMetadata: Partial<ErrorMetadata> = {};

  private constructor() {
    // Singleton pattern
  }

  /**
   * Get the ErrorTrackingService instance
   */
  public static getInstance(): ErrorTrackingService {
    if (!ErrorTrackingService.instance) {
      ErrorTrackingService.instance = new ErrorTrackingService();
    }
    return ErrorTrackingService.instance;
  }

  /**
   * Initialize error tracking service with global handlers
   */
  public init(defaultMetadata: Partial<ErrorMetadata> = {}): void {
    if (this.isInitialized) {
      console.warn('ErrorTrackingService already initialized');
      return;
    }

    this.defaultMetadata = defaultMetadata;
    this.setupGlobalHandlers();
    this.isInitialized = true;

    // ErrorTrackingService initialized
  }

  /**
   * Add error handler
   * @param handler - Error handler function
   */
  public addHandler(handler: ErrorHandler): void {
    this.handlers.push(handler);
  }

  /**
   * Remove error handler
   * @param handler - Error handler function to remove
   */
  public removeHandler(handler: ErrorHandler): void {
    this.handlers = this.handlers.filter(h => h !== handler);
  }

  /**
   * Manually capture and report an error
   * @param error - Error to capture
   * @param metadata - Additional error metadata
   */
  public captureError(error: Error, metadata?: ErrorMetadata): void {
    const fullMetadata = {
      ...this.defaultMetadata,
      ...metadata,
      url: metadata?.url || (typeof window !== 'undefined' ? window.location.href : undefined),
      timestamp: new Date().toISOString(),
    };

    // Log to console in development
    if (NODE_ENV === 'development') {
      console.error('Error captured:', error);
      console.error('Error metadata:', fullMetadata);
    }

    // Call all registered handlers
    this.handlers.forEach(handler => {
      try {
        handler(error, fullMetadata);
      } catch (handlerError) {
        console.error('Error in error handler:', handlerError);
      }
    });
  }

  /**
   * Setup global error handlers for uncaught exceptions
   */
  private setupGlobalHandlers(): void {
    if (typeof window !== 'undefined') {
      // Handle uncaught exceptions
      window.addEventListener('error', event => {
        this.captureError(event.error || new Error(`Unhandled error: ${event.message}`), {
          type: 'uncaught_exception',
          filename: event.filename,
          lineno: event.lineno,
          colno: event.colno,
        });
      });

      // Handle unhandled promise rejections
      window.addEventListener('unhandledrejection', event => {
        const error =
          typeof event.reason === 'object' && event.reason instanceof Error
            ? event.reason
            : new Error(`Unhandled Promise rejection: ${String(event.reason)}`);

        this.captureError(error, { type: 'unhandled_rejection' });
      });
    }
  }
}

// Export singleton instance
export const errorTracking = ErrorTrackingService.getInstance();

// Example integration with an external service
export function setupErrorTracking(
  userId?: string,
  customMetadata?: Record<string, unknown>
): void {
  // Initialize with default metadata
  errorTracking.init({
    userId,
    environment: NODE_ENV,
    release: APP_VERSION,
    ...customMetadata,
  });

  // Example: Add handler to send errors to an external service
  errorTracking.addHandler((_error, _metadata) => {
    // In a real application, this would send the error to a service like Sentry, LogRocket, etc.
    // Development mode: would send to error tracking service
    // Example code for integration with an external service:
    /*
    if (typeof window !== 'undefined' && window.ExternalErrorService) {
      window.ExternalErrorService.captureException(error, {
        extra: metadata
      });
    }
    */
  });
}
