'use client';

import React from 'react';
import type {
  BuiltinToolsResponse,
  BuiltinToolProvider,
  BuiltinToolItem,
} from '@/services/types/tool';
import type { Locale } from '@/lib/i18n';
import { ToolCatalogList } from '../node-catalog';

export interface ToolsTabProps {
  tools: BuiltinToolsResponse | undefined;
  isLoading: boolean;
  isFetching: boolean;
  labels: { toolsEmpty: string };
  locale: Locale;
  onAddTool: (provider: BuiltinToolProvider, tool: BuiltinToolItem) => void;
}

/**
 * @component ToolsTab
 * @category Workflow
 * @status Stable
 * @description Left-panel builtin tool catalog with drag-to-create behavior
 * @usage Use inside the workflow left node panel
 * @example
 * <ToolsTab tools={tools} labels={labels} onAddTool={addTool} />
 */
const ToolsTab: React.FC<ToolsTabProps> = ({
  tools,
  isLoading,
  isFetching,
  labels,
  locale,
  onAddTool,
}) => {
  return (
    <ToolCatalogList
      tools={tools}
      isLoading={isLoading}
      isFetching={isFetching}
      labels={labels}
      locale={locale}
      onSelect={onAddTool}
      density="panel"
      interaction="drag"
      tooltipSide="right"
    />
  );
};

export default ToolsTab;
