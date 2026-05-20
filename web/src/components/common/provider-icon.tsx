'use client';

import { ProviderIcon as LobeProviderIcon } from '@lobehub/icons';
import type { ComponentProps } from 'react';
import { resolveProviderIconKey } from '@/utils/provider/meta';

type LobeProviderIconProps = ComponentProps<typeof LobeProviderIcon>;

export interface ProviderIconProps extends Omit<LobeProviderIconProps, 'provider'> {
  provider?: string | null;
}

export function ProviderIcon({ provider, ...props }: ProviderIconProps): JSX.Element {
  const mappedProvider = resolveProviderIconKey(provider);
  return <LobeProviderIcon provider={mappedProvider} {...props} />;
}
