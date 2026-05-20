'use client';

import { useMutation } from '@tanstack/react-query';
import { WebAppService } from '@/services/webapp.service';
import type { WebAppRunRequest } from '@/services/types/webapp';

/**
 * @hook useWebAppPrecheck
 * @description Runs the published webapp precheck before console-side executions.
 */
export function useWebAppPrecheck(versionUuid: string) {
  return useMutation({
    mutationFn: (payload: WebAppRunRequest) => WebAppService.precheck(versionUuid, payload),
  });
}
