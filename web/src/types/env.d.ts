// Global typing for runtime public environment injected into the browser
// Only public variables (prefixed with NEXT_PUBLIC_) are exposed.

declare global {
  interface Window {
    __ENV__?: Record<string, string>;
  }
}

export {};
