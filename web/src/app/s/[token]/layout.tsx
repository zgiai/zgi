import type { ReactNode } from 'react';
import { PublicRouteProviders } from '@/providers/public-route-providers';

export default function ArtifactShareLayout({ children }: { children: ReactNode }) {
  return <PublicRouteProviders>{children}</PublicRouteProviders>;
}
