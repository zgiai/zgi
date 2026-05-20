'use client';

import React from 'react';
import { useT } from '@/i18n';
import { FileText, Upload, Globe, Plus, ArrowRight, Sparkles, Database } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';

interface EmptyElementProps {
  type?: 'documents' | 'search' | 'history' | 'generic';
  title?: string;
  description?: string;
  illustration?: React.ReactNode;
  actions?: Array<{
    label: string;
    icon?: React.ReactNode;
    onClick: () => void;
    variant?: 'default' | 'outline' | 'ghost';
    highlight?: boolean;
  }>;
  suggestions?: Array<{
    title: string;
    description: string;
    icon: React.ReactNode;
    badge?: string;
    onClick?: () => void;
  }>;
  className?: string;
}

// Default illustrations for different types
const DefaultIllustrations = {
  documents: (
    <div className="relative">
      <div className="w-20 h-20 bg-gradient-to-br from-blue-100 to-blue-200 rounded-2xl flex items-center justify-center">
        <FileText className="h-10 w-10 text-blue-600" />
      </div>
      <div className="absolute -top-2 -right-2 w-6 h-6 bg-green-500 rounded-full flex items-center justify-center">
        <Plus className="h-3 w-3 text-white" />
      </div>
    </div>
  ),
  search: (
    <div className="w-20 h-20 bg-gradient-to-br from-gray-100 to-gray-200 rounded-2xl flex items-center justify-center">
      <Database className="h-10 w-10 text-gray-600" />
    </div>
  ),
  history: (
    <div className="w-20 h-20 bg-gradient-to-br from-purple-100 to-purple-200 rounded-2xl flex items-center justify-center">
      <Sparkles className="h-10 w-10 text-purple-600" />
    </div>
  ),
  generic: (
    <div className="w-20 h-20 bg-gradient-to-br from-indigo-100 to-indigo-200 rounded-2xl flex items-center justify-center">
      <FileText className="h-10 w-10 text-indigo-600" />
    </div>
  ),
};

export function EmptyElement({
  type = 'generic',
  title,
  description,
  illustration,
  actions = [],
  suggestions = [],
  className,
}: EmptyElementProps) {
  const t = useT();

  // Default content based on type
  const getDefaultContent = () => {
    switch (type) {
      case 'documents':
        return {
          title: title || t('datasets.empty.noDocuments'),
          description: description || t('datasets.empty.noDocumentsDesc'),
          illustration: illustration || DefaultIllustrations.documents,
          suggestions:
            suggestions.length > 0
              ? suggestions
              : [
                  {
                    title: t('datasets.documents.uploadFile'),
                    description: t('datasets.empty.uploadFileDesc'),
                    icon: <Upload className="h-5 w-5" />,
                    badge: t('datasets.common.recommended'),
                    onClick: undefined,
                  },
                  {
                    title: t('datasets.documents.syncNotion'),
                    description: t('datasets.empty.syncNotionDesc'),
                    icon: <FileText className="h-5 w-5" />,
                    onClick: undefined,
                  },
                  {
                    title: t('datasets.documents.crawlWebsite'),
                    description: t('datasets.empty.crawlWebsiteDesc'),
                    icon: <Globe className="h-5 w-5" />,
                    onClick: undefined,
                  },
                ],
        };
      case 'search':
        return {
          title: title || t('datasets.empty.noResults'),
          description: description || t('datasets.empty.noResultsDesc'),
          illustration: illustration || DefaultIllustrations.search,
        };
      case 'history':
        return {
          title: title || t('datasets.empty.noHistory'),
          description: description || t('datasets.empty.noHistoryDesc'),
          illustration: illustration || DefaultIllustrations.history,
        };
      default:
        return {
          title: title || t('datasets.empty.empty'),
          description: description || t('datasets.empty.emptyDesc'),
          illustration: illustration || DefaultIllustrations.generic,
        };
    }
  };

  const content = getDefaultContent();

  return (
    <div
      className={cn('flex flex-col items-center justify-center py-12 px-6 text-center', className)}
    >
      {/* Illustration */}
      <div className="mb-6">{content.illustration}</div>

      {/* Title and Description */}
      <div className="max-w-md space-y-2 mb-8">
        <h3 className="text-lg font-semibold text-foreground">{content.title}</h3>
        <p className="text-sm text-muted-foreground leading-relaxed">{content.description}</p>
      </div>

      {/* Primary Actions */}
      {actions.length > 0 && (
        <div className="flex flex-wrap gap-3 mb-8">
          {actions.map((action, index) => (
            <Button
              key={index}
              variant={action.variant || 'default'}
              onClick={action.onClick}
              className={`flex items-center gap-2 ${action.highlight ? 'bg-highlight text-primary-foreground hover:bg-highlight/80' : ''}`}
            >
              {action.icon}
              {action.label}
            </Button>
          ))}
        </div>
      )}

      {/* Suggestions */}
      {content.suggestions && content.suggestions.length > 0 && (
        <div className="w-full max-w-2xl">
          <div className="text-sm font-medium text-muted-foreground mb-4">
            {t('datasets.empty.suggestions')}
          </div>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {content.suggestions.map((suggestion, index) => (
              <Card
                key={index}
                className={cn(
                  'cursor-pointer transition-all duration-200 hover:shadow-md hover:border-primary/20',
                  suggestion.onClick && 'hover:bg-accent/50'
                )}
                onClick={suggestion.onClick}
              >
                <CardHeader className="pb-3">
                  <div className="flex items-start justify-between">
                    <div className="flex items-center gap-3">
                      <div className="p-2 bg-primary/10 rounded-lg">{suggestion.icon}</div>
                      <div className="text-left">
                        <CardTitle className="text-sm font-medium">{suggestion.title}</CardTitle>
                        {suggestion.badge && (
                          <Badge variant="secondary" className="mt-1 text-xs">
                            {suggestion.badge}
                          </Badge>
                        )}
                      </div>
                    </div>
                    {suggestion.onClick && <ArrowRight className="h-4 w-4 text-muted-foreground" />}
                  </div>
                </CardHeader>
                <CardContent className="pt-0">
                  <CardDescription className="text-xs text-left">
                    {suggestion.description}
                  </CardDescription>
                </CardContent>
              </Card>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// Predefined empty state components for common use cases
export function EmptyDocuments({
  onUploadFile,
  onSyncNotion,
  onCrawlWebsite,
  className,
}: {
  onUploadFile?: () => void;
  onSyncNotion?: () => void;
  onCrawlWebsite?: () => void;
  className?: string;
}) {
  const t = useT('datasets');

  const actions = [];
  if (onUploadFile) {
    actions.push({
      label: t('documents.uploadFile'),
      icon: <Upload className="h-4 w-4" />,
      onClick: onUploadFile,
      primary: true,
    });
  }

  const suggestions = [
    onUploadFile && {
      title: t('documents.uploadFile'),
      description: t('empty.uploadFileDesc'),
      icon: <Upload className="h-5 w-5" />,
      badge: t('common.recommended'),
      onClick: onUploadFile,
    },
    onSyncNotion && {
      title: t('documents.syncNotion'),
      description: t('empty.syncNotionDesc'),
      icon: <FileText className="h-5 w-5" />,
      onClick: onSyncNotion,
    },
    onCrawlWebsite && {
      title: t('documents.crawlWebsite'),
      description: t('empty.crawlWebsiteDesc'),
      icon: <Globe className="h-5 w-5" />,
      onClick: onCrawlWebsite,
    },
  ].filter(Boolean) as EmptyElementProps['suggestions'];

  return (
    <EmptyElement
      type="documents"
      actions={actions}
      suggestions={suggestions}
      className={className}
    />
  );
}

export function EmptySearchResults({
  query,
  onClearSearch,
  className,
}: {
  query?: string;
  onClearSearch?: () => void;
  className?: string;
}) {
  const t = useT();

  const actions = onClearSearch
    ? [
        {
          label: t('datasets.messages.clearFilters'),
          onClick: onClearSearch,
          variant: 'outline' as const,
        },
      ]
    : [];

  return (
    <EmptyElement
      type="search"
      title={query ? t('datasets.empty.noResultsFor', { query }) : t('datasets.empty.noResults')}
      description={t('datasets.empty.noResultsDesc')}
      actions={actions}
      className={className}
    />
  );
}

export function EmptyTestHistory({
  onStartTesting,
  className,
}: {
  onStartTesting?: () => void;
  className?: string;
}) {
  const t = useT();

  const actions = onStartTesting
    ? [
        {
          label: t('datasets.messages.startTesting'),
          icon: <Sparkles className="h-4 w-4" />,
          onClick: onStartTesting,
          highlight: true,
        },
      ]
    : [];

  return <EmptyElement type="history" actions={actions} className={className} />;
}
