'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { IS_CLOUD } from '@/lib/config';

/**
 * @hook useCloudOnlyPage
 * @description Redirects non-cloud users away from cloud-only pages.
 */
export function useCloudOnlyPage(redirectTo: string = '/dashboard'): boolean {
  const router = useRouter();

  useEffect(() => {
    if (!IS_CLOUD) {
      router.replace(redirectTo);
    }
  }, [redirectTo, router]);

  return IS_CLOUD;
}
