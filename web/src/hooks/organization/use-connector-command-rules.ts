'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { connectorService } from '@/services/connector.service';
import type { CreateConnectorCommandRuleRequest } from '@/services/types/connector';

const commandRulesKey = ['connector-command-rules'] as const;

/** The organization's server command rules (ADR 0029). */
export function useConnectorCommandRules(enabled: boolean) {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: commandRulesKey,
    queryFn: () => connectorService.getCommandRules(),
    enabled,
    staleTime: 10_000,
    retry: false,
  });
  const invalidate = () => queryClient.invalidateQueries({ queryKey: commandRulesKey });
  const create = useMutation({
    mutationFn: (request: CreateConnectorCommandRuleRequest) =>
      connectorService.createCommandRule(request),
    onSuccess: invalidate,
  });
  const remove = useMutation({
    mutationFn: (ruleId: string) => connectorService.deleteCommandRule(ruleId),
    onSuccess: invalidate,
  });
  return { query, create, remove };
}
