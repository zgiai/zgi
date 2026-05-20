'use client';

import { useEffect } from 'react';
import { installDomMutationGuard } from '@/lib/dom-mutation-guard';

export function DomMutationGuard() {
  useEffect(() => {
    installDomMutationGuard();
  }, []);

  return null;
}
