'use client';

import { useCallback, useEffect, useRef } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { organizationService } from '@/services/organization.service';
import { accountService } from '@/services/account.service';
import type { Organization } from '@/services/types/organization';
import { toast } from 'sonner';
import { useOrganizationStore } from '@/store/organization-store';
import { useAuthStore } from '@/store/auth-store';
import { useWorkspaceStore } from '@/store/workspace-store';
import { useT } from '@/i18n';
import { ORGANIZATION_KEYS, WORKSPACE_KEYS } from '../query-keys';
import { sessionManager } from '@/lib/auth/session-manager';
import { clearProfileClientCache } from '@/utils/client-cache';

interface UseOrganizationsResult {
  organizations: Organization[];
  currentOrganization: Organization | null;
  isLoading: boolean;
  isFetching: boolean;
  reload: () => Promise<void>;
  switchOrganization: (organization: Organization) => Promise<void>;
}

/**
 * React hook to manage organizations for the current user.
 */
export function useOrganizations(autoLoad: boolean = true): UseOrganizationsResult {
  const t = useT();

  const queryClient = useQueryClient();

  const organizations = useOrganizationStore.use.organizations();
  const currentOrganization = useOrganizationStore.use.currentOrganization();
  const setOrganizations = useOrganizationStore.use.setOrganizations();
  const setCurrentOrganization = useOrganizationStore.use.setCurrentOrganization();
  const resetWorkspaceForOrganizationSwitch =
    useWorkspaceStore.use.resetForOrganizationSwitch();
  const isAuthenticated = useAuthStore.use.isAuthenticated();
  const isLoggingOut = useAuthStore.use.isLoggingOut();
  const shouldAutoLoad = autoLoad && isAuthenticated && !isLoggingOut;

  const hasErrorProcessed = useRef(false);

  /* ----------------------------- Data fetching ----------------------------- */
  // Fetch organizations list
  const {
    data: fetchedOrganizations,
    isLoading: listLoading,
    isFetching: listFetching,
    error: listError,
  } = useQuery({
    queryKey: ORGANIZATION_KEYS.list({ page: 1, limit: 100 }),
    enabled: shouldAutoLoad,
    staleTime: 5 * 60 * 1000,
    queryFn: async () => {
      const list = await organizationService.getOrganizationList({ page: 1, limit: 100 });
      return list.data;
    },
  });

  useEffect(() => {
    if (fetchedOrganizations) {
      setOrganizations(fetchedOrganizations);
    }
  }, [fetchedOrganizations, setOrganizations]);

  // Fetch current organization
  const { data: fetchedCurrentOrganization, error: currentOrganizationError } = useQuery({
    queryKey: ORGANIZATION_KEYS.current(),
    enabled: shouldAutoLoad,
    staleTime: 5 * 60 * 1000,
    refetchInterval: 5 * 60 * 1000,
    queryFn: async () => {
      return await organizationService.getCurrentOrganization();
    },
  });

  useEffect(() => {
    if (fetchedCurrentOrganization) {
      setCurrentOrganization(fetchedCurrentOrganization);
    }
  }, [fetchedCurrentOrganization, setCurrentOrganization]);

  // Error handling
  useEffect(() => {
    if (
      (listError || currentOrganizationError) &&
      !hasErrorProcessed.current &&
      shouldAutoLoad
    ) {
      hasErrorProcessed.current = true;
      toast.error(t('common.organization.fetchOrgFailed'));
    }
  }, [listError, currentOrganizationError, t, shouldAutoLoad]);

  // If still no currentOrganization
  useEffect(() => {
    if (shouldAutoLoad && !currentOrganization) {
      queryClient.invalidateQueries({ queryKey: ORGANIZATION_KEYS.current() });
    }
  }, [currentOrganization, queryClient, shouldAutoLoad]);

  // Sync reference
  useEffect(() => {
    if (currentOrganization && organizations.length > 0) {
      const matched = organizations.find(g => g.id === currentOrganization.id);
      if (matched && matched !== currentOrganization) {
        setCurrentOrganization(matched);
      }
    }
  }, [currentOrganization, organizations, setCurrentOrganization]);

  const reload = useCallback(async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ORGANIZATION_KEYS.lists() }),
      queryClient.invalidateQueries({ queryKey: ORGANIZATION_KEYS.current() }),
    ]);
  }, [queryClient]);

  const switchOrganization = useCallback(
    async (organization: Organization) => {
      if (currentOrganization?.id === organization.id) {
        return;
      }
      try {
        await accountService.updateContext({ current_organization_id: organization.id });
        resetWorkspaceForOrganizationSwitch();
        queryClient.removeQueries({ queryKey: WORKSPACE_KEYS.all });
        clearProfileClientCache();
        setCurrentOrganization(organization);
        try {
          await useAuthStore.getState().refreshProfile({ refresh: true });
          // Invalidate ALL queries because organization context change affects everything
          await queryClient.invalidateQueries();
        } catch {
          // ignore
        }
        sessionManager.broadcastContextChanged({
          currentOrganizationId: organization.id,
          currentWorkspaceId: null,
        });
        toast.success(t('common.organization.switchOrgSuccess'));
      } catch {
        toast.error(t('common.organization.switchOrgFailed'));
      }
    },
    [
      currentOrganization,
      resetWorkspaceForOrganizationSwitch,
      setCurrentOrganization,
      t,
      queryClient,
    ]
  );

  return {
    organizations,
    currentOrganization,
    isLoading: listLoading,
    isFetching: listFetching,
    reload,
    switchOrganization,
  };
}
