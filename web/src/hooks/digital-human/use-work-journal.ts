'use client';

import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { digitalHumanService } from '@/services/digital-human.service';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import { useWorkspaceStore } from '@/store/workspace-store';
import {
  sameWorkJournalScope,
  workJournalQueryKey,
  type WorkJournalScope,
} from '@/components/digital-human/digital-human-work-journal-model';

function currentScope(employeeId: string): WorkJournalScope | undefined {
  const accountId = useAuthStore.getState().user?.id;
  const organizationId = useOrganizationStore.getState().currentOrganization?.id;
  const workspaceId = useWorkspaceStore.getState().currentWorkspace?.id;
  return accountId && organizationId && workspaceId
    ? { accountId, organizationId, workspaceId, employeeId }
    : undefined;
}

export function useWorkJournal(
  employeeId: string,
  enabled: boolean,
  recordId?: string,
  operationId?: string
) {
  const account = useAuthStore.use.user();
  const organization = useOrganizationStore.use.currentOrganization();
  const workspace = useWorkspaceStore.use.currentWorkspace();
  const scope =
    account?.id && organization?.id && workspace?.id
      ? {
          accountId: account.id,
          organizationId: organization.id,
          workspaceId: workspace.id,
          employeeId,
        }
      : undefined;
  const queryKey = scope
    ? workJournalQueryKey(scope)
    : ['digital-humans', 'work-journal', 'disabled'];
  const canFetch = enabled && Boolean(scope);
  const guarded = async <T>(request: () => Promise<T>) => {
    if (!scope || !sameWorkJournalScope(scope, currentScope(employeeId))) {
      throw new Error('work journal scope changed');
    }
    const result = await request();
    if (!sameWorkJournalScope(scope, currentScope(employeeId))) {
      throw new Error('work journal scope changed');
    }
    return result;
  };
  const records = useInfiniteQuery({
    queryKey: [...queryKey, 'records'],
    queryFn: ({ pageParam }) =>
      guarded(() =>
        digitalHumanService.listWorkJournalRecords(employeeId, { after_id: pageParam, limit: 20 })
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: last => last.data.next_after_id ?? undefined,
    enabled: canFetch,
    retry: false,
  });
  const operations = useInfiniteQuery({
    queryKey: [...queryKey, 'operations'],
    queryFn: ({ pageParam }) =>
      guarded(() =>
        digitalHumanService.listWorkJournalOperations(employeeId, {
          after_id: pageParam,
          limit: 20,
        })
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: last => last.data.next_after_id ?? undefined,
    enabled: canFetch,
    retry: false,
  });
  const record = useQuery({
    queryKey: [...queryKey, 'record', recordId],
    queryFn: () =>
      guarded(() => digitalHumanService.getWorkJournalRecord(employeeId, recordId ?? '')),
    enabled: canFetch && Boolean(recordId),
    retry: false,
  });
  const history = useInfiniteQuery({
    queryKey: [...queryKey, 'history', recordId],
    queryFn: ({ pageParam }) =>
      guarded(() =>
        digitalHumanService.getWorkJournalRecordHistory(employeeId, recordId ?? '', {
          after_revision: pageParam,
          limit: 20,
        })
      ),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: last => last.data.next_after_revision ?? undefined,
    enabled: canFetch && Boolean(recordId),
    retry: false,
  });
  const operation = useQuery({
    queryKey: [...queryKey, 'operation', operationId],
    queryFn: () =>
      guarded(() => digitalHumanService.getWorkJournalOperation(employeeId, operationId ?? '')),
    enabled: canFetch && Boolean(operationId),
    retry: false,
  });
  return { records, operations, record, history, operation };
}
