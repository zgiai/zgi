import { useQuery, useMutation } from '@tanstack/react-query';
import { inviteService } from '@/services/invite.service';

export const useInviteInfo = (token: string) => {
  return useQuery({
    queryKey: ['inviteInfo', token],
    queryFn: () => inviteService.getInviteInfo(token),
    enabled: !!token,
    retry: false,
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
};

export const useAcceptInvite = () => {
  return useMutation({
    mutationFn: ({ token, member_name }: { token: string; member_name?: string }) =>
      inviteService.acceptInvite(token, member_name),
  });
};
