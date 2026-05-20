import React from 'react';
import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';

interface GraphLegendProps {
  title: string;
  categories: any[];
  categoryColorMap: Record<string, { fill: string; stroke: string; text: string }>;
  sources: { id: string; title: string }[];
  selectedSourceIds: string[];
  onSelectedSourcesChange: (ids: string[]) => void;
}

export const GraphLegend: React.FC<GraphLegendProps> = ({
  title,
  categories,
  categoryColorMap,
  sources,
  selectedSourceIds,
  onSelectedSourcesChange,
}) => {
  return (
    <div className="absolute top-4 left-4 z-10 bg-background/80 backdrop-blur-md p-4 rounded-xl border border-border shadow-2xl text-xs space-y-4 w-60 opacity-0 group-hover:opacity-100 transition-all duration-300">
      <div>
        <div className="font-semibold text-sm border-b border-border/50 pb-2 mb-2">{title}</div>
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
          <div className="font-semibold text-sm border-b border-border/50 pb-2 mb-2">来源文档</div>
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
                全选
              </Label>
            </div>

            <div className="space-y-1.5 pt-1">
              {sources.map(source => (
                <div key={source.id} className="flex items-center space-x-2">
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
                    className="text-[11px] truncate max-w-[140px] cursor-pointer opacity-70"
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

      <div className="text-[10px] text-muted-foreground pt-2 border-t border-border/50 italic opacity-60">
        提示：节点大小代表该实体在选中背景下的权重。
      </div>
    </div>
  );
};
