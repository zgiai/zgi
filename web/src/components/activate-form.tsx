'use client';

import { FormEvent, useEffect, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Icons } from '@/components/ui/icons';
import { Input, PasswordInput } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { authService } from '@/services/auth.service';
import type { ActivationCheckResponse } from '@/services/types/auth';
import { useAuthStore } from '@/store/auth-store';
import { getErrorMessage } from '@/utils/error-notifications';

const ActivateForm = () => {
  const router = useRouter();
  const searchParams = useSearchParams();
  const workspaceID = searchParams.get('workspace_id') || undefined;
  const email = searchParams.get('email') || '';
  const token = searchParams.get('token') || '';
  const [result, setResult] = useState<ActivationCheckResponse | null>(null);
  const [name, setName] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!email || !token) {
      setResult({ is_valid: false, status: 'invalid_or_expired' });
      setLoading(false);
      return;
    }
    void authService.checkActivate({ email, token, workspace_id: workspaceID }).then(res => {
      setResult(res);
      setName(res.data?.user_name || '');
    }).catch(() => setResult({ is_valid: false, status: 'invalid_or_expired' })).finally(() => setLoading(false));
  }, [email, token, workspaceID]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (password !== confirmPassword) {
      setError('两次输入的密码不一致');
      return;
    }
    setSubmitting(true);
    setError('');
    try {
      await authService.activate({
        token,
        email,
        workspace_id: result?.data?.workspace_id || workspaceID,
        name,
        password,
        interface_language: navigator.language.startsWith('zh') ? 'zh-Hans' : 'en-US',
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
      });
      await useAuthStore.getState().initializeAuth({ force: true });
      router.replace('/console');
    } catch (cause) {
      setError(getErrorMessage(cause));
    } finally {
      setSubmitting(false);
    }
  };

  const statusMessage = result?.status === 'already_activated'
    ? '该邀请已完成注册，请直接登录。'
    : result?.status === 'organization_unavailable'
      ? '受邀组织已停用或不存在，请联系邀请人。'
    : '邀请链接无效、已过期或已被使用，请联系邀请人重新发送。';

  return (
    <div className="flex w-full grow items-center justify-center px-6 md:px-[108px]">
      <Card className="w-full max-w-md">
        <CardHeader><CardTitle className="text-center text-2xl">接受邀请并注册</CardTitle></CardHeader>
        <CardContent className="space-y-5">
          {loading && <div className="flex justify-center py-8"><Icons.Spinner className="h-6 w-6 animate-spin" /></div>}
          {!loading && !result?.is_valid && (
            <Alert variant="destructive"><Icons.AlertCircle className="h-4 w-4" /><AlertDescription>{statusMessage}</AlertDescription></Alert>
          )}
          {!loading && result?.is_valid && result.data && (
            <form className="space-y-4" onSubmit={submit}>
              {result.data.workspace_name && (
                <Alert><AlertDescription>你将加入{result.data.organization_name ? `组织「${result.data.organization_name}」的` : ''}工作区：<strong>{result.data.workspace_name}</strong></AlertDescription></Alert>
              )}
              <div className="space-y-2"><Label htmlFor="invite-email">受邀邮箱</Label><Input id="invite-email" value={result.data.email} disabled /></div>
              <div className="space-y-2"><Label htmlFor="invite-name">姓名</Label><Input id="invite-name" value={name} onChange={e => setName(e.target.value)} maxLength={30} required /></div>
              <div className="space-y-2"><Label htmlFor="invite-password">设置密码</Label><PasswordInput id="invite-password" value={password} onChange={e => setPassword(e.target.value)} minLength={8} required /></div>
              <div className="space-y-2"><Label htmlFor="invite-password-confirm">确认密码</Label><PasswordInput id="invite-password-confirm" value={confirmPassword} onChange={e => setConfirmPassword(e.target.value)} minLength={8} required /></div>
              {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
              <Button className="w-full" type="submit" disabled={submitting}>{submitting && <Icons.Spinner className="mr-2 h-4 w-4 animate-spin" />}加入并进入工作区</Button>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  );
};

export default ActivateForm;
