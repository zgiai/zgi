'use client';

import type { MouseEvent } from 'react';
import { usePathname } from 'next/navigation';
import { BrainCircuit, History, House, ListTodo, RadioTower, Settings2 } from 'lucide-react';
import {
  ResourceSidebar,
  ResourceSidebarHeader,
  type ResourceSidebarNavItem,
} from '@/components/common/resource-sidebar';
import { useDigitalHuman } from '@/hooks/digital-human/use-digital-humans';
import { usePersistentSidebarCollapse } from '@/hooks/use-persistent-sidebar-collapse';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';

interface DigitalHumanSidebarProps {
  id: string;
  onNavigate: (event: MouseEvent<HTMLAnchorElement>, href: string) => void;
}

export function DigitalHumanSidebar({ id, onNavigate }: DigitalHumanSidebarProps) {
  const pathname = usePathname();
  const t = useT();
  const detail = useDigitalHuman(id);
  const permissions = useAccountPermissions();
  const [isCollapsed, setIsCollapsed] = usePersistentSidebarCollapse('digitalHuman', true);
  const configurationHref = `/console/digital-humans/${id}/configuration`;
  const navItems: ResourceSidebarNavItem[] = [
    {
      title: t('digitalHumans.tabs.dashboard'),
      href: `/console/digital-humans/${id}`,
      icon: House,
      isActive: currentPathname => currentPathname === `/console/digital-humans/${id}`,
    },
    {
      title: t('digitalHumans.tabs.configuration'),
      href: configurationHref,
      icon: Settings2,
      isActive: currentPathname =>
        currentPathname.startsWith(`/console/digital-humans/${id}/configuration`),
    },
    {
      title: t('digitalHumans.tabs.channels'),
      href: `/console/digital-humans/${id}/channels`,
      icon: RadioTower,
    },
    {
      title: t('digitalHumans.tabs.tasks'),
      href: `/console/digital-humans/${id}/tasks`,
      icon: ListTodo,
    },
    {
      title: t('digitalHumans.tabs.runs'),
      href: `/console/digital-humans/${id}/runs`,
      icon: History,
    },
    ...(permissions.hasPermission('digital_human.memory.view')
      ? [
          {
            title: t('digitalHumans.tabs.memory'),
            href: `/console/digital-humans/${id}/memory`,
            icon: BrainCircuit,
          },
        ]
      : []),
  ];

  return (
    <ResourceSidebar
      isCollapsed={isCollapsed}
      onToggleCollapse={() => setIsCollapsed(value => !value)}
      expandLabel={t('navigation.expand')}
      collapseLabel={t('navigation.collapse')}
      expandedWidthClassName="w-44"
      header={
        <ResourceSidebarHeader
          isCollapsed={isCollapsed}
          isLoading={detail.isLoading}
          iconBackground="var(--primary)"
          name={detail.data?.data.definition.name}
          description={detail.data?.data.definition.description}
          showIdentity={false}
          backHref="/console/digital-humans"
          backLabel={t('digitalHumans.sidebar.backToList')}
          onBackClick={event => onNavigate(event, '/console/digital-humans')}
        />
      }
      navItems={navItems}
      pathname={pathname}
      onNavigate={onNavigate}
    />
  );
}
