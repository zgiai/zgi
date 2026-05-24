'use client';

import { useEffect, useState } from 'react';
import { ZgiLoadingScreen } from '@/components/brand/zgi-loading-screen';
import { useSetupStatus } from '@/hooks/use-setup';
import { withBasePathIfInternal } from '@/lib/config';
import { AuthRouteProviders } from '@/providers/auth-route-providers';
import { useAuthLoading, useIsAuthenticated, useIsInitialized } from '@/store/auth-store';

export default function HomePage() {
  return (
    <AuthRouteProviders>
      <HomePageContent />
    </AuthRouteProviders>
  );
}

function HomePageContent() {
  const [isMounted, setIsMounted] = useState(false);
  const { isInitialized: isSetupInitialized, isLoading: isSetupLoading } = useSetupStatus();
  const isAuthInitialized = useIsInitialized();
  const isAuthLoading = useAuthLoading();
  const isAuthenticated = useIsAuthenticated();

  useEffect(() => {
    setIsMounted(true);
  }, []);

  useEffect(() => {
    if (isSetupLoading || isAuthLoading || !isAuthInitialized) return;

    if (!isSetupInitialized) {
      window.location.replace(withBasePathIfInternal('/init'));
      return;
    }

    if (isAuthenticated) {
      window.location.replace(withBasePathIfInternal('/console'));
    } else {
      window.location.replace(withBasePathIfInternal('/login'));
    }
  }, [isAuthInitialized, isAuthLoading, isAuthenticated, isSetupInitialized, isSetupLoading]);

  const phase =
    !isMounted || isSetupLoading
      ? 'setup'
      : isAuthLoading || !isAuthInitialized
        ? 'auth'
        : 'routing';

  return <ZgiLoadingScreen phase={phase} />;
}
