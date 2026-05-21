'use client';

import { ProviderIcon as ModeliconsProviderIcon } from 'modelicons';
import type { ComponentProps } from 'react';
import { resolveProviderIconKey } from '@/utils/provider/meta';

type ModeliconsProviderIconProps = ComponentProps<typeof ModeliconsProviderIcon>;

export interface ProviderIconProps extends Omit<ModeliconsProviderIconProps, 'provider'> {
  provider?: string | null;
}

export function ProviderIcon({ provider, ...props }: ProviderIconProps): JSX.Element {
  const mappedProvider = resolveProviderIconKey(provider);
  return <ModeliconsProviderIcon provider={mappedProvider} {...props} />;
}
