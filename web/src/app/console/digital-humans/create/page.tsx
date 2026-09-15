import { Suspense } from 'react';
import { DigitalHumanCreateFlow } from '@/components/digital-human/create/digital-human-create-flow';
import { DigitalHumanFeatureGate } from '@/components/digital-human/feature-gate';

/**
 * Dedicated creation route.
 *
 * This is a static route (unlike `[digitalHumanId]`), so reading the `role`
 * query parameter has to sit behind a Suspense boundary, otherwise the build
 * fails on the `useSearchParams` prerender requirement.
 */
export default function DigitalHumanCreatePage() {
  return (
    <DigitalHumanFeatureGate permission="digital_human.create">
      <Suspense fallback={null}>
        <DigitalHumanCreateFlow />
      </Suspense>
    </DigitalHumanFeatureGate>
  );
}
