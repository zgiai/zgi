'use client';

import { useEffect, useMemo, useState } from 'react';
import { AlertTriangle, Check, ChevronsUpDown, Search } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { useCurrentOrganizationMember } from '@/hooks/organization/use-current-organization-member';
import { useCurrentOrganizationMembers } from '@/hooks/organization/use-current-organization-members';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { IntegrationConnectionGrantPrincipalType } from '@/services/types/integration';
import type { Member } from '@/services/types/organization';
import { safeIntegrationDisplayText, safeOptionalIntegrationDisplayText } from './display-utils';

interface PrincipalOption {
  id: string;
  label: string;
}

interface GrantPrincipalPickerProps {
  principalType: Exclude<IntegrationConnectionGrantPrincipalType, 'organization'>;
  value: string;
  initialLabel?: string | null;
  initialState?: 'active' | 'missing';
  workspaces: PrincipalOption[];
  workspacesLoading?: boolean;
  onChange: (value: string, label: string) => void;
}

function memberName(member: Member, fallback: string): string {
  return safeIntegrationDisplayText(member.member_name || member.name || member.email, fallback);
}

function memberSecondaryLabel(member: Member, name: string): string | null {
  const email = safeOptionalIntegrationDisplayText(member.email);
  return email && email !== name ? email : null;
}

export function GrantPrincipalPicker({
  principalType,
  value,
  initialLabel,
  initialState,
  workspaces,
  workspacesLoading = false,
  onChange,
}: GrantPrincipalPickerProps) {
  const t = useT('integrations');
  const [open, setOpen] = useState(false);
  const [keyword, setKeyword] = useState('');
  const safeInitialLabel = safeOptionalIntegrationDisplayText(initialLabel);
  const [selectedLabel, setSelectedLabel] = useState(() =>
    safeIntegrationDisplayText(safeInitialLabel, '')
  );
  const debouncedKeyword = useDebouncedValue(keyword.trim(), 300);
  const normalizedValue = value.trim();

  const membersQuery = useCurrentOrganizationMembers({
    keyword: debouncedKeyword,
    limit: 100,
    enabled: open && principalType === 'account',
  });
  const selectedMemberQuery = useCurrentOrganizationMember(normalizedValue, {
    enabled: principalType === 'account' && Boolean(normalizedValue),
  });
  const activeMembers = useMemo(
    () => membersQuery.members.filter(member => member.status === 'active'),
    [membersQuery.members]
  );

  const filteredWorkspaces = useMemo(() => {
    const normalizedKeyword = debouncedKeyword.toLocaleLowerCase();
    if (!normalizedKeyword) return workspaces;
    return workspaces.filter(workspace =>
      workspace.label.toLocaleLowerCase().includes(normalizedKeyword)
    );
  }, [debouncedKeyword, workspaces]);

  const selectedWorkspace = useMemo(
    () => workspaces.find(workspace => workspace.id === normalizedValue),
    [normalizedValue, workspaces]
  );
  const selectedSearchMember = useMemo(
    () => activeMembers.find(member => member.id === normalizedValue),
    [activeMembers, normalizedValue]
  );
  const hydratedMember =
    selectedMemberQuery.member?.status === 'active' ? selectedMemberQuery.member : null;
  const selectedMemberIsInactive =
    principalType === 'account' &&
    Boolean(selectedMemberQuery.member) &&
    selectedMemberQuery.member?.status !== 'active';

  useEffect(() => {
    if (safeInitialLabel) setSelectedLabel(safeInitialLabel);
  }, [safeInitialLabel]);

  useEffect(() => {
    if (principalType === 'workspace' && selectedWorkspace) {
      setSelectedLabel(
        safeIntegrationDisplayText(
          selectedWorkspace.label,
          t('grants.principalPicker.unnamed.workspace')
        )
      );
      return;
    }
    if (principalType === 'account' && selectedSearchMember) {
      setSelectedLabel(
        memberName(selectedSearchMember, t('grants.principalPicker.unnamed.account'))
      );
      return;
    }
    if (principalType === 'account' && hydratedMember) {
      setSelectedLabel(memberName(hydratedMember, t('grants.principalPicker.unnamed.account')));
      return;
    }
    if (!normalizedValue) setSelectedLabel('');
  }, [hydratedMember, normalizedValue, principalType, selectedSearchMember, selectedWorkspace, t]);

  useEffect(() => {
    if (!open) setKeyword('');
  }, [open]);

  const lookupPending =
    principalType === 'account' &&
    Boolean(normalizedValue) &&
    (selectedMemberQuery.isLoading || selectedMemberQuery.isFetching);
  const unresolved =
    Boolean(normalizedValue) &&
    (initialState === 'missing' ||
      selectedMemberIsInactive ||
      (principalType === 'workspace'
        ? !workspacesLoading && !selectedWorkspace && !selectedLabel && !safeInitialLabel
        : !lookupPending &&
          !selectedSearchMember &&
          !hydratedMember &&
          !selectedLabel &&
          !safeInitialLabel &&
          Boolean(selectedMemberQuery.error)));
  const displayLabel = lookupPending
    ? t('grants.principalPicker.resolving')
    : unresolved
      ? t(`grants.principalPicker.missing.${principalType}`)
      : selectedLabel ||
        safeInitialLabel ||
        t(`grants.principalPicker.placeholder.${principalType}`);
  const optionsLoading =
    principalType === 'account'
      ? membersQuery.isLoading || membersQuery.isFetching
      : workspacesLoading;
  const hasOptions =
    principalType === 'account' ? activeMembers.length > 0 : filteredWorkspaces.length > 0;

  return (
    <div className="space-y-2">
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            type="button"
            variant="outline"
            role="combobox"
            aria-expanded={open}
            aria-invalid={unresolved || undefined}
            aria-label={t(`grants.principalPicker.label.${principalType}`)}
            className={cn(
              'w-full justify-between font-normal',
              !normalizedValue && 'text-muted-foreground',
              unresolved &&
                'border-amber-300 text-amber-700 dark:border-amber-900 dark:text-amber-300'
            )}
          >
            <span className="truncate text-left">{displayLabel}</span>
            <ChevronsUpDown className="size-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent align="start" className="w-[var(--radix-popover-trigger-width)] p-2">
          <Input
            value={keyword}
            onChange={event => setKeyword(event.target.value)}
            placeholder={t(`grants.principalPicker.search.${principalType}`)}
            aria-label={t(`grants.principalPicker.search.${principalType}`)}
            leftIcon={<Search />}
          />
          <div className="mt-2 max-h-60 overflow-y-auto" role="listbox">
            {optionsLoading ? (
              <div className="px-2 py-3 text-sm text-muted-foreground" aria-live="polite">
                {t('grants.principalPicker.loading')}
              </div>
            ) : membersQuery.error && principalType === 'account' ? (
              <div className="px-2 py-3 text-sm text-destructive" role="alert">
                {t('grants.principalPicker.loadFailed')}
              </div>
            ) : !hasOptions ? (
              <div className="px-2 py-3 text-sm text-muted-foreground">
                {t(`grants.principalPicker.empty.${principalType}`)}
              </div>
            ) : principalType === 'account' ? (
              activeMembers.map(member => {
                const label = memberName(member, t('grants.principalPicker.unnamed.account'));
                const secondary = memberSecondaryLabel(member, label);
                const selected = member.id === normalizedValue;
                return (
                  <button
                    key={member.id}
                    type="button"
                    role="option"
                    aria-selected={selected}
                    className={cn(
                      'flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm hover:bg-accent hover:text-accent-foreground',
                      selected && 'bg-accent text-accent-foreground'
                    )}
                    onClick={() => {
                      setSelectedLabel(label);
                      onChange(member.id, label);
                      setOpen(false);
                    }}
                  >
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-medium">{label}</span>
                      {secondary ? (
                        <span className="block truncate text-xs text-muted-foreground">
                          {secondary}
                        </span>
                      ) : null}
                    </span>
                    {selected ? <Check className="size-4 shrink-0 text-primary" /> : null}
                  </button>
                );
              })
            ) : (
              filteredWorkspaces.map(workspace => {
                const selected = workspace.id === normalizedValue;
                const label = safeIntegrationDisplayText(
                  workspace.label,
                  t('grants.principalPicker.unnamed.workspace')
                );
                return (
                  <button
                    key={workspace.id}
                    type="button"
                    role="option"
                    aria-selected={selected}
                    className={cn(
                      'flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm hover:bg-accent hover:text-accent-foreground',
                      selected && 'bg-accent text-accent-foreground'
                    )}
                    onClick={() => {
                      setSelectedLabel(label);
                      onChange(workspace.id, label);
                      setOpen(false);
                    }}
                  >
                    <span className="min-w-0 flex-1 truncate font-medium">{label}</span>
                    {selected ? <Check className="size-4 shrink-0 text-primary" /> : null}
                  </button>
                );
              })
            )}
          </div>
        </PopoverContent>
      </Popover>

      {unresolved ? (
        <div
          className="flex items-start gap-1.5 rounded-md border border-amber-300/70 bg-amber-50/60 px-2 py-1.5 text-xs text-amber-700 dark:border-amber-900/70 dark:bg-amber-950/20 dark:text-amber-300"
          role="alert"
        >
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
          <span>{t(`grants.principalPicker.missingDescription.${principalType}`)}</span>
        </div>
      ) : null}
    </div>
  );
}
