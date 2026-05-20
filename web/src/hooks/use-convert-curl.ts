'use client';

import { useMutation } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { parseCurlToRequest, type ConvertCurlResult } from '@/utils/curl';
import { normalizeToastDescription } from '@/utils/error-notifications';

// Types aligned with ConvertCurlResult
export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH' | 'HEAD';
export interface HttpHeaderKV {
  key: string;
  value: string;
}

interface ConvertCurlVars {
  curl: string;
}

interface UseConvertCurlOptions {
  // Optional external success handler to consume parsed result
  onSuccess?: (data: ConvertCurlResult) => void;
}

/**
 * Hook to convert a cURL command into structured HTTP request data on the client side.
 * - Uses TanStack Query useMutation to manage request lifecycle
 * - Shows success/error toasts inside the hook
 * - Avoids backend API and native dependencies, ensuring stability across environments
 */
export function useConvertCurl(options: UseConvertCurlOptions = {}) {
  const t = useT('nodes');

  return useMutation<ConvertCurlResult, Error, ConvertCurlVars>({
    mutationFn: async (vars: ConvertCurlVars) => {
      if (!vars.curl || typeof vars.curl !== 'string') {
        throw new Error(t('httpRequest.toasts.missingCurl'));
      }
      // Pure TS parsing
      try {
        return parseCurlToRequest(vars.curl);
      } catch {
        throw new Error(t('httpRequest.toasts.parseFailedDesc'));
      }
    },
    onSuccess: data => {
      toast(t('httpRequest.toasts.imported'), {
        description: t('httpRequest.toasts.importedDesc'),
      });
      options.onSuccess?.(data);
    },
    onError: (error: Error) => {
      const title = t('httpRequest.toasts.importFailed');
      const description = error.message || t('httpRequest.toasts.parseFailedDesc');
      toast.error(title, {
        description: normalizeToastDescription(title, description),
      });
    },
  });
}
