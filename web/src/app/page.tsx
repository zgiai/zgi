'use client';

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { ZgiLoadingScreen } from '@/components/brand/zgi-loading-screen';
import { useSetupStatus } from '@/hooks/use-setup';
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
  const router = useRouter();
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
      router.replace('/init');
      return;
    }

    if (isAuthenticated) {
      router.replace('/console');
    } else {
      router.replace('/login');
    }
  }, [isAuthInitialized, isAuthLoading, isAuthenticated, isSetupInitialized, isSetupLoading, router]);

  const phase =
    !isMounted || isSetupLoading
      ? 'setup'
      : isAuthLoading || !isAuthInitialized
        ? 'auth'
        : 'routing';

  return <ZgiLoadingScreen phase={phase} />;
}
