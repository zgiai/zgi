import type { ReactNode } from 'react';
import { PublicRouteProviders } from '@/providers/public-route-providers';

export default function AnnouncementTokenLayout({ children }: { children: ReactNode }) {
  return <PublicRouteProviders>{children}</PublicRouteProviders>;
}
