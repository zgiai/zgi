'use client';

import { useState } from 'react';
import { BookOpen, Bot, Workflow, FileText } from 'lucide-react';
import { useT } from '@/i18n';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';
import { fileManageService } from '@/services/file-manage.service';
import type { RelatedResource, RelatedResourceItem } from '@/services/types/file';
import { Button } from '@/components/ui/button';
import { useRouter } from 'next/navigation';

interface RelatedResourcesPopoverProps {
  fileId: string;
  relatedCount: number;
  children: React.ReactNode;
}

/**
 * Get icon for resource type
 */
function getResourceTypeIcon(type: RelatedResourceItem['type']) {
  switch (type) {
    case 'dataset':
      return BookOpen;
    case 'agent':
      return Bot;
    case 'workflow':
      return Workflow;
    default:
      return FileText;
  }
}

/**
 * Get color for resource type
 */
function getResourceTypeColor(type: RelatedResourceItem['type']): string {
  switch (type) {
    case 'dataset':
      return 'text-blue-700 ';
    case 'agent':
      return 'text-green-700 ';
    case 'workflow':
      return ' text-purple-700 ';
    default:
      return ' text-gray-700 ';
  }
}

export function RelatedResourcesPopover({
  fileId,
  relatedCount: _relatedCount,
  children,
}: RelatedResourcesPopoverProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [resources, setResources] = useState<RelatedResource | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const t = useT('files');
  const router = useRouter();

  const fetchRelatedResources = async () => {
    if (resources) return; // Already loaded

    setIsLoading(true);
    setError(null);

    try {
      const response = await fileManageService.getRelatedResources(fileId);
      setResources(response.data.data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load related resources');
    } finally {
      setIsLoading(false);
    }
  };

  const handleOpenChange = (open: boolean) => {
    setIsOpen(open);
    if (open && !resources) {
      fetchRelatedResources();
    }
  };

  // Calculate total count and items
  const allItems: RelatedResourceItem[] = [
    ...(resources?.datasets?.items?.map(item => ({
      ...item,
      type: 'dataset' as const,
    })) || []),
  ];

  return (
    <div onClick={e => e.stopPropagation()}>
      <DropdownMenu open={isOpen} onOpenChange={handleOpenChange}>
        <DropdownMenuTrigger asChild onClick={e => e.stopPropagation()}>
          {children}
        </DropdownMenuTrigger>
        <DropdownMenuContent className="w-80 p-0" align="start">
          <div className="p-4">
            <div className="flex items-center gap-2 mb-3">
              <h4 className="font-medium">{t('relatedResources.title')}</h4>
            </div>

            <div className="max-h-64 overflow-y-auto">
              {isLoading ? (
                <div className="space-y-2">
                  {Array.from({ length: 3 }).map((_, i) => (
                    <div key={i} className="flex items-center gap-3 p-2">
                      <Skeleton className="h-8 w-8 rounded" />
                      <div className="flex-1 space-y-1">
                        <Skeleton className="h-4 w-3/4" />
                        <Skeleton className="h-3 w-1/2" />
                      </div>
                    </div>
                  ))}
                </div>
              ) : error ? (
                <div className="text-center py-4">
                  <p className="text-sm text-destructive mb-2">{error}</p>
                  <Button variant="outline" size="sm" onClick={fetchRelatedResources}>
                    {t('relatedResources.retry')}
                  </Button>
                </div>
              ) : allItems.length === 0 ? (
                <div className="text-center py-4">
                  <p className="text-sm text-muted-foreground">{t('relatedResources.empty')}</p>
                </div>
              ) : (
                <div className="space-y-2">
                  {allItems.map(resource => {
                    const Icon = getResourceTypeIcon(resource.type);
                    const typeColor = getResourceTypeColor(resource.type);

                    return (
                      <div
                        key={resource.id}
                        className="flex items-center gap-3 p-2 rounded-lg justify-between"
                      >
                        <div className="flex items-center">
                          <div className={cn('p-1.5 ', typeColor)}>
                            <Icon className="h-4 w-4" />
                          </div>
                          <div className="ml-2"> {resource.name}</div>
                        </div>

                        <Button
                          variant="link"
                          className="text-sm text-blue-500"
                          onClick={e => {
                            e.stopPropagation();
                            router.push(`/console/dataset/${resource.id}`);
                          }}
                        >
                          {t('relatedResources.open')}
                        </Button>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </div>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
