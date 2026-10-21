'use client';

import type { ReactNode } from 'react';
import { Providers } from '@/providers';

export default function DigitalHumanWebAppLayout({ children }: { children: ReactNode }) {
  return <Providers>{children}</Providers>;
}
