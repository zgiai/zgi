'use client';

import { useQuery } from '@tanstack/react-query';

import { announcementService } from '@/services/announcement.service';

export const announcementKeys = {
  all: ['announcements'] as const,
  detail: (token: string) => [...announcementKeys.all, token] as const,
};

export function useAnnouncement(token: string | null | undefined, enabled = true) {
  return useQuery({
    queryKey: announcementKeys.detail(token || ''),
    queryFn: () => announcementService.getAnnouncement(token || ''),
    enabled: enabled && Boolean(token),
    retry: false,
  });
}
