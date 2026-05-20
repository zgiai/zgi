'use client';

import { Bot } from 'lucide-react';
import { AIChatSkillSettingsSection } from '@/components/dashboard/organization/aichat-skill-settings-section';
import { useT } from '@/i18n';

/**
 * @component OrganizationAIChatSkillsPage
 * @category Page
 * @status Stable
 * @description Organization management page for AIChat Skill enablement.
 * @usage Route page for /dashboard/organization/aichat-skills
 * @example
 * <OrganizationAIChatSkillsPage />
 */
export default function OrganizationAIChatSkillsPage() {
  const t = useT('dashboard');

  return (
    <div className="h-full overflow-y-auto bg-bg-canvas/50 p-4 lg:p-6">
      <div className="mx-auto mt-4 w-full max-w-[1440px]">
        <AIChatSkillSettingsSection />
      </div>
    </div>
  );
}
