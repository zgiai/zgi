// Type augmentation for next-intl
// This enables type-safe translations with IDE auto-completion for default hooks

import type { Messages } from './modules';

declare module 'next-intl' {
  interface AppConfig {
    Messages: Messages;
  }
}
