import { Suspense } from 'react';
import { SSOCallbackHandler } from '@/components/auth/sso-callback-handler';

export default function SSOCallbackPage() {
  return (
    <Suspense fallback={null}>
      <SSOCallbackHandler />
    </Suspense>
  );
}
