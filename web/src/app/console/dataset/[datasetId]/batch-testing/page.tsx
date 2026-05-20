'use client';

import { Suspense } from 'react';
import BatchTesting from '@/components/datasets/batch-testing';

export default function BatchTestingPage() {
  return (
    <Suspense fallback={null}>
      <BatchTesting />
    </Suspense>
  );
}
