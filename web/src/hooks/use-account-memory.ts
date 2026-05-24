import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { MEMORY_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n/translations';
import { memoryService } from '@/services/memory.service';
import type {
  CreateAccountMemoryEntryRequest,
  UpdateAccountMemoryEntryRequest,
  UpdateAccountMemorySettingRequest,
} from '@/services/types/memory';

export function useAccountMemory() {
  return useQuery({
    queryKey: MEMORY_KEYS.me(),
    queryFn: async () => {
      const response = await memoryService.getMe();
      return response.data;
    },
    retry: false,
  });
}

export function useUpdateAccountMemorySettings() {
  const queryClient = useQueryClient();
  const t = useT('webapp');
  return useMutation({
    mutationFn: (payload: UpdateAccountMemorySettingRequest) => memoryService.updateSettings(payload),
    onSuccess: async response => {
      queryClient.setQueryData(MEMORY_KEYS.me(), response.data);
    },
    onError: error => {
      toast.error(error instanceof Error ? error.message : t('consoleChat.memory.saveFailed'));
    },
  });
}

export function useCreateAccountMemoryEntry() {
  const queryClient = useQueryClient();
  const t = useT('webapp');
  return useMutation({
    mutationFn: (payload: CreateAccountMemoryEntryRequest) => memoryService.createEntry(payload),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: MEMORY_KEYS.me() });
    },
    onError: error => {
      toast.error(error instanceof Error ? error.message : t('consoleChat.memory.saveFailed'));
    },
  });
}

export function useUpdateAccountMemoryEntry() {
  const queryClient = useQueryClient();
  const t = useT('webapp');
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: UpdateAccountMemoryEntryRequest }) =>
      memoryService.updateEntry(id, payload),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: MEMORY_KEYS.me() });
    },
    onError: error => {
      toast.error(error instanceof Error ? error.message : t('consoleChat.memory.saveFailed'));
    },
  });
}

export function useDeleteAccountMemoryEntry() {
  const queryClient = useQueryClient();
  const t = useT('webapp');
  return useMutation({
    mutationFn: (id: string) => memoryService.deleteEntry(id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: MEMORY_KEYS.me() });
    },
    onError: error => {
      toast.error(error instanceof Error ? error.message : t('consoleChat.memory.deleteFailed'));
    },
  });
}
