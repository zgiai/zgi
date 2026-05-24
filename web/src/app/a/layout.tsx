import type { ReactNode } from 'react';
import { AuthRouteProviders } from '@/providers/auth-route-providers';

export default function ApprovalTokenLayout({ children }: { children: ReactNode }) {
  return <AuthRouteProviders>{children}</AuthRouteProviders>;
}
