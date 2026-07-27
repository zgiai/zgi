'use client';

import { Switch } from '@/components/ui/switch';
import { ProviderIcon } from '@/components/common/provider-icon';
import { ChevronLeft, Edit, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import Link from 'next/link';
import { useT } from '@/i18n';

interface ProviderPageHeaderProps {
  providerId?: string;
  displayName?: string;
  description: string;
  isEnabled: boolean;
  onToggle: (next: boolean) => void;
  toggling: boolean;
  onEdit?: () => void;
  onDelete?: () => void;
}

export default function ProviderPageHeader({
  providerId,
  displayName,
  description,
  isEnabled,
  onToggle,
  toggling,
  onEdit,
  onDelete,
}: ProviderPageHeaderProps): JSX.Element {
  const t = useT('aiProviders');

  return (
    <div className="flex items-center justify-between">
      <div className="flex items-center gap-3">
        <Link href="/dashboard/provider" className="rounded-md p-1 hover:bg-muted">
          <ChevronLeft size={24} />
        </Link>
        <ProviderIcon provider={providerId || displayName} size={40} />
        <div>
          <div className="text-lg font-medium">{displayName || providerId}</div>
          <div className="text-sm text-muted-foreground">{description}</div>
        </div>
      </div>
      <div className="flex items-center gap-3">
        {onEdit && (
          <Button
            variant="ghost"
            isIcon
            className="h-8 w-8 text-muted-foreground hover:text-primary"
            onClick={onEdit}
          >
            <Edit className="h-4 w-4" />
          </Button>
        )}
        {onDelete && (
          <Button
            variant="ghost"
            isIcon
            className="h-8 w-8 text-muted-foreground hover:text-destructive"
            onClick={onDelete}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        )}
        <span className="hidden text-xs text-muted-foreground sm:inline">
          {t('management.organizationAccess')}
        </span>
        <Switch
          checked={isEnabled}
          onCheckedChange={checked => onToggle(checked as boolean)}
          disabled={toggling}
        />
      </div>
    </div>
  );
}
