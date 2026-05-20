'use client';
import { useEffect, useState } from 'react';
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Icons } from '@/components/ui/icons';
import { useRouter, useSearchParams } from 'next/navigation';
import { useT } from '@/i18n';
import { authService } from '@/services/auth.service';
import type { ActivationCheckResponse } from '@/services/types/auth';
import { ROUTES } from '@/constants/routes';
import { getErrorMessage } from '@/utils/error-notifications';

const ActivateForm = () => {
  const router = useRouter();
  const t = useT('auth');
  const searchParams = useSearchParams();

  // Extract params from URL
  const workspaceID = searchParams.get('workspace_id') || undefined;
  const email = searchParams.get('email') || undefined;
  const token = searchParams.get('token') || undefined;

  // State for loading, error, and result
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<ActivationCheckResponse | null>(null);

  useEffect(() => {
    // Validate required params
    if (!email || !token) {
      setError(t('activateLinkExpired'));
      setLoading(false);
      return;
    }
    // Call activation check API
    setLoading(true);
    setError(null);
    authService
      .checkActivate({ email, token, workspace_id: workspaceID })
      .then(res => {
        setResult(res);
        if (res.is_valid && res.data) {
          // Redirect to login with params if valid
          const params = new URLSearchParams();
          params.set('email', encodeURIComponent(email));
          if (workspaceID) params.set('workspace_id', encodeURIComponent(workspaceID));
          params.set('invite_token', encodeURIComponent(token));
          router.replace(`${ROUTES.AUTH.LOGIN}?${params.toString()}`);
        } else {
          setLoading(false);
        }
      })
      .catch(err => {
        console.error('Activation check failed:', getErrorMessage(err));
        setError(t('activateLinkExpired'));
        setLoading(false);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="flex flex-col items-center w-full grow justify-center px-6 md:px-[108px]">
      <Card className="w-full max-w-md">
        <CardHeader className="text-center">
          <CardTitle className="text-2xl font-bold">
            {t('activateLinkTitle', { defaultValue: 'Activate Account' })}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-6">
          {loading && (
            <div className="flex justify-center py-8">
              <Icons.Spinner className="mr-2 h-6 w-6 animate-spin" />
            </div>
          )}
          {!loading && error && (
            <Alert variant="destructive">
              <Icons.AlertCircle className="h-4 w-4" />
              <AlertDescription>{t('activateLinkExpired')}</AlertDescription>
            </Alert>
          )}
          {/* If not loading, not error, but not valid, show invalid message */}
          {!loading && !error && result && !result.is_valid && (
            <Alert variant="destructive">
              <Icons.AlertCircle className="h-4 w-4" />
              <AlertDescription>{t('activateLinkExpired')}</AlertDescription>
            </Alert>
          )}
        </CardContent>
      </Card>
    </div>
  );
};

export default ActivateForm;
