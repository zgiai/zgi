'use client';

import React, { useMemo } from 'react';
import { Network, Info, FileText, ChevronRight } from 'lucide-react';
import { Card } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { ScrollArea } from '@/components/ui/scroll-area';
import { cn } from '@/lib/utils';
import type { DatasetGraph, GraphNode } from '@/services/types/dataset';

interface DetailPanelProps {
  selectedNode: GraphNode | null;
  graphData: DatasetGraph | null;
  categoryColorMap: Record<string, { fill: string; stroke: string; text: string }>;
  onNodeSelect: (nodeId: string) => void;
  className?: string;
}

export const DetailPanel: React.FC<DetailPanelProps> = ({
  selectedNode,
  graphData,
  categoryColorMap,
  onNodeSelect,
  className,
}) => {
  // Compute related entities (neighbors) and their relationships
  const relatedEntities = useMemo(() => {
    if (!selectedNode || !graphData) return [];

    const neighbors = graphData.edges
      .filter(edge => edge.source === selectedNode.id || edge.target === selectedNode.id)
      .map(edge => {
        const isSource = edge.source === selectedNode.id;
        const neighborId = isSource ? edge.target : edge.source;
        const neighborNode = graphData.nodes.find(n => n.id === neighborId);

        return {
          edge,
          node: neighborNode,
          direction: isSource ? 'out' : 'in',
        };
      })
      .filter(item => !!item.node);

    return neighbors;
  }, [selectedNode, graphData]);

  if (!selectedNode) {
    return (
      <Card className={cn('flex flex-col overflow-hidden bg-card', className)}>
        <div className="h-full flex flex-col items-center justify-center p-8 text-center text-muted-foreground">
          <Network className="w-12 h-12 mb-4 opacity-10" />
          <p className="text-sm">在图谱中点击实体以查看详细信息</p>
        </div>
      </Card>
    );
  }

  return (
    <Card className={cn('flex flex-col overflow-hidden bg-card', className)}>
      <div className="p-4 border-b border-border bg-muted/30">
        <h3 className="font-semibold flex items-center gap-2">
          <Info className="w-4 h-4" />
          实体详情
        </h3>
      </div>

      <ScrollArea className="flex-1">
        <div className="p-4 space-y-6">
          {/* Node Basic Info */}
          <div>
            <Badge
              variant="outline"
              className="mb-2"
              style={
                categoryColorMap[selectedNode.category]
                  ? {
                      backgroundColor: categoryColorMap[selectedNode.category].fill,
                      color: categoryColorMap[selectedNode.category].text,
                      borderColor: categoryColorMap[selectedNode.category].stroke,
                    }
                  : {}
              }
            >
              {selectedNode.category}
            </Badge>
            <h2 className="text-xl font-bold">{selectedNode.label}</h2>
          </div>

          {/* Description */}
          {selectedNode.data?.description && (
            <div className="space-y-2">
              <h4 className="text-xs font-bold text-muted-foreground uppercase tracking-wider">
                描述
              </h4>
              <p className="text-sm leading-relaxed text-foreground/90">
                {selectedNode.data.description}
              </p>
            </div>
          )}

          {/* Related Entities Section */}
          <div className="space-y-3">
            <h4 className="text-xs font-bold text-muted-foreground uppercase tracking-wider">
              关联实体 ({relatedEntities.length})
            </h4>
            {relatedEntities.length > 0 ? (
              <div className="space-y-2">
                {relatedEntities.map((item, idx) => (
                  <div
                    key={`${item.edge.source}-${item.edge.target}-${idx}`}
                    onClick={() => onNodeSelect(item.node!.id)}
                    className="p-3 rounded-lg border border-border bg-muted/20 hover:bg-muted/40 transition-all cursor-pointer group"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2 mb-1">
                          <Badge
                            variant="secondary"
                            className="text-[10px] px-1.5 h-4 bg-primary/10 text-primary border-none"
                          >
                            {item.edge.label}
                          </Badge>
                          <span className="text-[10px] text-muted-foreground italic">
                            {item.direction === 'out' ? '→' : '←'}
                          </span>
                        </div>
                        <p className="text-sm font-medium truncate group-hover:text-primary transition-colors">
                          {item.node!.label}
                        </p>
                        <p className="text-[10px] text-muted-foreground">{item.node!.category}</p>
                      </div>
                      <ChevronRight className="w-4 h-4 text-muted-foreground/30 group-hover:text-primary transition-colors shrink-0" />
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-xs text-muted-foreground italic">暂无关联实体</p>
            )}
          </div>

          {/* Source Documents */}
          {selectedNode.data?.sources && selectedNode.data.sources.length > 0 && (
            <div className="space-y-3">
              <h4 className="text-xs font-bold text-muted-foreground uppercase tracking-wider">
                来源文档 ({selectedNode.data.sources.length})
              </h4>
              <div className="space-y-2">
                {selectedNode.data.sources.map((src: any) => (
                  <div
                    key={src.doc.id}
                    className="p-3 rounded-lg border border-border bg-muted/20 hover:bg-muted/40 transition-colors cursor-pointer group"
                  >
                    <div className="flex items-start gap-2">
                      <FileText className="w-4 h-4 mt-1 shrink-0 text-muted-foreground group-hover:text-primary" />
                      <div className="min-w-0">
                        <p className="text-sm font-medium line-clamp-1 truncate">{src.doc.title}</p>
                        <div className="flex items-center gap-2 mt-1">
                          <div className="flex-1 h-1 bg-muted rounded-full overflow-hidden">
                            <div
                              className="h-full bg-primary"
                              style={{ width: `${src.weight}%` }}
                            />
                          </div>
                          <span className="text-[10px] text-muted-foreground shrink-0">
                            {src.weight}%
                          </span>
                        </div>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </ScrollArea>
    </Card>
  );
};
