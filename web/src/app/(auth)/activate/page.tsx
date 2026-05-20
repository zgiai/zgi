import { Suspense } from 'react';
import ActivateForm from '@/components/activate-form';

export default async function Activate() {
  return (
    <Suspense fallback={null}>
      <ActivateForm />
    </Suspense>
  );
}
