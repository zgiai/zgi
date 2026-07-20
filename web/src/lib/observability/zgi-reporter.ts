import { sanitizeZGIEvent } from './sanitize';
import type { Reporter, ZGIEvent } from './types';

/**
 * ZGIReporter is the global provider-neutral facade. With no registered
 * adapters it is a No-op; with multiple adapters it fans out to all of them.
 */
export class ZGIReporter {
  private readonly reporters = new Map<string, Reporter>();

  register(reporter: Reporter): void {
    const name = reporter.name.trim().toLowerCase();
    if (!name || this.reporters.has(name)) return;
    this.reporters.set(name, reporter);
  }

  unregister(name: string): void {
    this.reporters.delete(name.trim().toLowerCase());
  }

  has(name: string): boolean {
    return this.reporters.has(name.trim().toLowerCase());
  }

  names(): string[] {
    return [...this.reporters.keys()];
  }

  report(event: ZGIEvent): void {
    if (this.reporters.size === 0) return;
    let sanitized: ZGIEvent;
    try {
      sanitized = sanitizeZGIEvent(event);
    } catch (error) {
      console.warn('ZGI Reporter discarded an event that could not be sanitized:', error);
      return;
    }
    for (const reporter of this.reporters.values()) {
      try {
        void Promise.resolve(reporter.report(sanitized)).catch(error => {
          console.warn(`ZGI Reporter adapter "${reporter.name}" failed:`, error);
        });
      } catch (error) {
        console.warn(`ZGI Reporter adapter "${reporter.name}" failed:`, error);
      }
    }
  }

  onRouterTransitionStart(...args: unknown[]): void {
    for (const reporter of this.reporters.values()) {
      try {
        reporter.onRouterTransitionStart?.(...args);
      } catch (error) {
        console.warn(`ZGI Reporter adapter "${reporter.name}" router hook failed:`, error);
      }
    }
  }

  async flush(): Promise<void> {
    await Promise.allSettled(
      [...this.reporters.values()].map(reporter => Promise.resolve(reporter.flush?.()))
    );
  }
}

export class NoopReporter implements Reporter {
  readonly name = 'noop';
  report(): void {}
}
