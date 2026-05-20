'use client';

import { useEffect, useState } from 'react';
import { useT } from '@/i18n';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input, PasswordInput } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import type { DepartmentMember } from '@/services/types/organization';

interface ResetMemberPasswordDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  member: DepartmentMember | null;
  onReset: (email: string, password?: string) => Promise<void>;
  isResetting?: boolean;
}

/**
 * @component ResetMemberPasswordDialog
 * @category Feature
 * @status Stable
 * @description Resets a current organization member password in non-cloud deployments.
 * @usage Open from the organization contacts member row actions.
 * @example
 * <ResetMemberPasswordDialog open={open} member={member} onReset={handleReset} />
 */
export function ResetMemberPasswordDialog({
  open,
  onOpenChange,
  member,
  onReset,
  isResetting = false,
}: ResetMemberPasswordDialogProps) {
  const t = useT('dashboard.organization.contacts.resetPassword');
  const [password, setPassword] = useState('');

  useEffect(() => {
    if (!open) {
      setPassword('');
    }
  }, [open]);

  const handleReset = async () => {
    if (!member?.account_email) return;

    const trimmedPassword = password.trim();
    await onReset(member.account_email, trimmedPassword || undefined);
    setPassword('');
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md p-0 flex flex-col overflow-hidden">
        <DialogHeader>
          <DialogTitle className="text-xl font-bold tracking-tight">{t('title')}</DialogTitle>
        </DialogHeader>

        <DialogBody className="min-h-0 space-y-5 overflow-y-auto">
          <div className="space-y-2">
            <Label htmlFor="reset-member-email" className="text-sm font-bold text-foreground ml-1">
              {t('email')}
            </Label>
            <Input
              id="reset-member-email"
              type="email"
              value={member?.account_email ?? ''}
              readOnly
              disabled
            />
          </div>

          <div className="space-y-2">
            <Label
              htmlFor="reset-member-password"
              className="text-sm font-bold text-foreground ml-1"
            >
              {t('password')}
            </Label>
            <PasswordInput
              id="reset-member-password"
              value={password}
              onChange={event => setPassword(event.target.value)}
              placeholder={t('passwordPlaceholder')}
              autoComplete="new-password"
              disabled={isResetting}
            />
            <p className="text-xs font-medium text-muted-foreground ml-1">
              {t('defaultPasswordHint')}
            </p>
          </div>
        </DialogBody>

        <DialogFooter className="bg-muted/50 px-6 pb-6 pt-4 border-t gap-3">
          <Button
            variant="ghost"
            size="xl"
            onClick={() => onOpenChange(false)}
            disabled={isResetting}
            className="px-6 font-semibold"
          >
            {t('cancel')}
          </Button>
          <Button
            size="xl"
            onClick={handleReset}
            loading={isResetting}
            disabled={!member?.account_email || isResetting}
            className="px-8 font-semibold"
          >
            {isResetting ? t('resetting') : t('confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
