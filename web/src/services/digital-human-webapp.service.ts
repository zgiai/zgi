import { http } from '@/lib/http';
import type { ApiResponseData } from '@/services/types/common';
import type { DigitalHumanWebAppConfig } from '@/services/types/digital-human';

export const digitalHumanWebAppService = {
  getConfig(webAppId: string): Promise<ApiResponseData<DigitalHumanWebAppConfig>> {
    return http.get(`/api/v1/digital-human-webapps/${encodeURIComponent(webAppId)}/config`, {
      skipAuth: true,
      skipErrorHandling: true,
    });
  },
};
