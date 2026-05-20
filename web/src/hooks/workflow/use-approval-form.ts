'use client';

import { useMutation, useQuery } from '@tanstack/react-query';

import {
  approvalService,
  type ApprovalSubmitRequest,
  type ApprovalEventItem,
} from '@/services/approval.service';

export const approvalFormKeys = {
  all: ['approval-forms'] as const,
  detail: (token: string) => [...approvalFormKeys.all, token] as const,
  events: (token: string) => [...approvalFormKeys.detail(token), 'events'] as const,
};

export function useApprovalForm(token: string | null | undefined, enabled = true) {
  return useQuery({
    queryKey: approvalFormKeys.detail(token || ''),
    queryFn: () => approvalService.getForm(token || ''),
    enabled: enabled && Boolean(token),
    retry: false,
  });
}

export function useSubmitApprovalForm(token: string | null | undefined) {
  return useMutation({
    mutationFn: (payload: ApprovalSubmitRequest) => {
      if (!token) throw new Error('Approval token is missing');
      return approvalService.submitForm(token, payload);
    },
  });
}

function normalizeEvents(value: unknown): ApprovalEventItem[] {
  if (Array.isArray(value)) return value as ApprovalEventItem[];
  if (value && typeof value === 'object') {
    const record = value as { data?: unknown; events?: unknown };
    if (Array.isArray(record.data)) return record.data as ApprovalEventItem[];
    if (Array.isArray(record.events)) return record.events as ApprovalEventItem[];
  }
  return [];
}

export async function fetchApprovalEvents(
  token: string,
  params: { after?: number; limit?: number } = {}
): Promise<ApprovalEventItem[]> {
  const response = await approvalService.getEvents(token, params);
  return normalizeEvents(response);
}
