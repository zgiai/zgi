import { BaseService } from '@/lib/http/services';
import type {
  SystemSettingsCategoriesResponse,
  SystemSettingsWithMetadata,
} from './types/system-settings';
import type { ApiResponseData } from './types/common';
import { getCurrentLocale } from '@/lib/i18n';

/**
 * System settings category enum
 */
export enum SystemSettingsCategory {
  /** CDN settings */
  CDN = 'cdn',
  /** Branding settings */
  BRANDING = 'branding',
  /** SMS settings */
  SMS = 'sms',
  /** Model settings */
  MODEL = 'model',
  /** Email settings */
  EMAIL = 'email',
  /** ETL/File parsing settings */
  ETL = 'etl',
  /** Datasource settings */
  DATASOURCE = 'datasource',
}

class SystemSettingsService extends BaseService {
  constructor() {
    super({
      basePath: '/console/api/system',
      endpoint: 'main',
    });
  }

  /**
   * Get all system settings categories
   * GET /console/api/system/settings?locale={locale}
   */
  getCategories(): Promise<ApiResponseData<SystemSettingsCategoriesResponse>> {
    return this.request('get', `/settings?locale=${getCurrentLocale()}`, undefined, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  /**
   * Get settings for a specific category with metadata
   * GET /console/api/system/settings/{category}/with-metadata?locale={locale}
   */
  getCategorySettings(
    category: SystemSettingsCategory
  ): Promise<ApiResponseData<SystemSettingsWithMetadata>> {
    return this.request(
      'get',
      `/settings/${category}/with-metadata?locale=${getCurrentLocale()}`,
      undefined,
      {
        headers: { 'Content-Type': 'application/json' },
      }
    );
  }

  /**
   * Update settings for a specific category
   * PUT /console/api/system/settings/{category}?locale={locale}
   */
  updateCategorySettings(
    category: SystemSettingsCategory,
    data: {
      settings: Record<string, unknown>;
      reason?: string;
    }
  ): Promise<ApiResponseData<void>> {
    return this.request('put', `/settings/${category}`, data, {
      headers: { 'Content-Type': 'application/json' },
    });
  }
}

export const systemSettingsService = new SystemSettingsService();
