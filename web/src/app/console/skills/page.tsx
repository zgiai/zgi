'use client';

import { AIChatSkillSettingsSection } from '@/components/dashboard/organization/aichat-skill-settings-section';
import { PageHeader } from '@/components/page-header';
import { useT } from '@/i18n';

export default function ConsoleSkillsPage() {
  const t = useT('dashboard');

  return (
    <div className="flex h-full flex-col overflow-y-auto bg-bg-canvas/50 p-4 sm:p-6 lg:p-8">
      <div className="mx-auto w-full max-w-[1440px] space-y-5">
        <PageHeader
          title={t('organization.aichatSkills.title')}
          description={t('organization.aichatSkills.description')}
        />
        <AIChatSkillSettingsSection />
      </div>
    </div>
  );
}
