'use client';

import { Suspense } from 'react';
import { VerificationForm } from '@/components/auth/verification-form';

export default function VerificationPage() {
  return (
    <Suspense fallback={<div>Loading...</div>}>
      <VerificationForm />
    </Suspense>
  );
}
