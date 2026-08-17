import React from 'react';
import { Eye, EyeOff } from 'lucide-react';
import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import type { GraphCategory } from '@/services/types/dataset';

interface GraphLegendProps {
  title: string;
  categories: GraphCategory[];
  categoryColorMap: Record<string, { fill: string; stroke: string; text: string }>;
  sources: Array<{ id: string; title: string }>;
  selectedSourceIds: string[];
  onSelectedSourcesChange: (ids: string[]) => void;
  hint?: string;
}

export const GraphLegend: React.FC<GraphLegendProps> = ({
  title,
  categories,
  categoryColorMap,
  sources,
  selectedSourceIds,
  onSelectedSourcesChange,
  hint,
}) => {
  const t = useT('datasets.knowledgeGraph');
  const [isHidden, setIsHidden] = React.useState(false);

  if (isHidden) {
    return (
      <Button
        type="button"
        variant="secondary"
        size="sm"
        onClick={() => setIsHidden(false)}
        className="absolute left-4 top-4 z-20 bg-background/90 shadow-md backdrop-blur-md"
      >
        <Eye className="mr-1.5 h-3.5 w-3.5" />
        {t('showLegend')}
      </Button>
    );
  }

  return (
    <div className="absolute left-4 top-4 z-10 max-h-[calc(100%-2rem)] w-60 space-y-4 overflow-y-auto rounded-xl border border-border bg-background/90 p-4 text-xs shadow-2xl backdrop-blur-md">
      <div>
        <div className="mb-2 flex items-center justify-between gap-2 border-b border-border/50 pb-2">
          <div className="font-semibold text-sm">{title}</div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => setIsHidden(true)}
            className="h-7 px-2 text-xs text-muted-foreground"
          >
            <EyeOff className="mr-1 h-3.5 w-3.5" />
            {t('hideLegend')}
          </Button>
        </div>
        <div className="grid grid-cols-2 gap-2 max-h-[120px] overflow-y-auto pr-1">
          {categories.map(cat => {
            const colors = categoryColorMap[cat.id] || { fill: '#F0F5FF', stroke: '#2F54EB' };
            return (
              <div key={cat.id} className="flex items-center gap-2">
                <div
                  className="w-2.5 h-2.5 rounded-full"
                  style={{ backgroundColor: colors.fill, border: `1px solid ${colors.stroke}` }}
                />
                <span className="truncate opacity-80">{cat.label['zh-Hans'] || cat.id}</span>
              </div>
            );
          })}
        </div>
      </div>

      {sources.length > 0 && (
        <div className="space-y-2">
          <div className="mb-2 border-b border-border/50 pb-2 text-sm font-semibold">
            {t('sourceDocuments')}
          </div>
          <div className="space-y-2 max-h-[180px] overflow-y-auto pr-1 custom-scrollbar">
            <div className="flex items-center space-x-2">
              <Checkbox
                id="select-all-sources"
                checked={selectedSourceIds.length === sources.length}
                onCheckedChange={checked =>
                  onSelectedSourcesChange(checked ? sources.map(s => s.id) : [])
                }
              />
              <Label htmlFor="select-all-sources" className="text-[11px] cursor-pointer">
                {t('selectAllSources')}
              </Label>
            </div>

            <div className="space-y-1.5 pt-1">
              {sources.map(source => (
                <div key={source.id} className="flex min-w-0 items-center space-x-2">
                  <Checkbox
                    id={`source-${source.id}`}
                    checked={selectedSourceIds.includes(source.id)}
                    onCheckedChange={checked => {
                      if (checked) {
                        onSelectedSourcesChange([...selectedSourceIds, source.id]);
                      } else {
                        onSelectedSourcesChange(selectedSourceIds.filter(id => id !== source.id));
                      }
                    }}
                  />
                  <Label
                    htmlFor={`source-${source.id}`}
                    className="min-w-0 flex-1 cursor-pointer truncate text-[11px] opacity-70"
                    title={source.title}
                  >
                    {source.title}
                  </Label>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {hint && (
        <div className="border-t border-border/50 pt-2 text-[10px] italic text-muted-foreground">
          {hint}
        </div>
      )}
    </div>
  );
};
