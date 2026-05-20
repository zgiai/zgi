'use client';

import React, { useRef, useState, useMemo, useEffect, useImperativeHandle } from 'react';
import { useT } from '@/i18n';
import type { DatasetGraph, GraphNode } from '@/services/types/dataset';
import { cn } from '@/lib/utils';
import { GraphCanvas } from './graph-canvas';
import { GraphLegend } from './graph-legend';
import { transformToG6Data } from './utils/data-adapter';
import { useGraphInstance } from './hooks/use-graph-instance';
import { getCategoryColorMap } from './utils/color';

export * from './detail-panel';

interface KnowledgeGraphProps {
  data: DatasetGraph;
  onNodeClick?: (node: GraphNode) => void;
  categoryColorMap: Record<string, { fill: string; stroke: string; text: string }>;
  className?: string;
}

export interface KnowledgeGraphHandle {
  focusNode: (nodeId: string) => void;
}

export const KnowledgeGraph = React.forwardRef<KnowledgeGraphHandle, KnowledgeGraphProps>(
  ({ data, onNodeClick, categoryColorMap, className }, ref) => {
    const containerRef = useRef<HTMLDivElement>(null);
    const t = useT().datasets;

    // 2. Source Selection Logic
    const allSources = useMemo(() => {
      const sourcesMap = new Map<string, string>();
      data.nodes.forEach(node => {
        node.data?.sources?.forEach((s: any) => {
          sourcesMap.set(s.doc.id, s.doc.title);
        });
      });
      return Array.from(sourcesMap.entries()).map(([id, title]) => ({ id, title }));
    }, [data]);

    const [selectedSourceIds, setSelectedSourceIds] = useState<string[]>(allSources.map(s => s.id));

    useEffect(() => {
      setSelectedSourceIds(allSources.map(s => s.id));
    }, [allSources]);

    // 3. Data Transformation
    const g6Data = useMemo(
      () => transformToG6Data(data, selectedSourceIds),
      [data, selectedSourceIds]
    );

    // 4. Initialize G6 Instance via Hook
    const graphInstanceRef = useGraphInstance({
      containerRef,
      data: g6Data,
      onNodeClick,
      categoryColorMap,
    });

    // 5. Expose handle
    useImperativeHandle(ref, () => ({
      focusNode: (nodeId: string) => {
        graphInstanceRef.current?.focusNode(nodeId);
      },
    }));

    return (
      <div className={cn('w-full h-full relative group', className)}>
        <GraphCanvas containerRef={containerRef} />

        <GraphLegend
          title={t('knowledgeGraphTitle')}
          categories={data.categories}
          categoryColorMap={categoryColorMap}
          sources={allSources}
          selectedSourceIds={selectedSourceIds}
          onSelectedSourcesChange={setSelectedSourceIds}
        />
      </div>
    );
  }
);

KnowledgeGraph.displayName = 'KnowledgeGraph';
