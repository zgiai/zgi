import React from 'react';
import type { JsonParserNodeData } from './config';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import CustomHandle from '../../ui/custom-handle';
import { Position } from '@xyflow/react';
import OutputVariablesView from '../../common/output-variables-view';
import { useNodeOutputVariables } from '../../hooks';

interface JsonParserContentProps {
  nodeId: string;
  data: JsonParserNodeData;
}

const JsonParserContent: React.FC<JsonParserContentProps> = ({ nodeId, data }) => {
  const t = useT('nodes');
  const inputConfigured = Array.isArray(data.input_selector) && data.input_selector.length >= 2;
  const outputKeys = Object.keys(data.outputs || {});
  const outputCount = outputKeys.length;
  const outputVariables = useNodeOutputVariables(nodeId);

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2 text-xs">
        <span className="text-muted-foreground">{t('jsonParser.labels.input')}:</span>
        {inputConfigured ? (
          <code className="bg-muted px-1.5 py-0.5 rounded text-xs font-mono truncate max-w-[160px]">
            {data.input_selector.join('.')}
          </code>
        ) : (
          <span className="text-muted-foreground italic">
            {t('jsonParser.labels.notConfigured')}
          </span>
        )}
      </div>

      <div className="flex items-center gap-2 text-xs">
        <span className="text-muted-foreground">{t('jsonParser.labels.outputs')}:</span>
        <Badge variant="secondary" className="text-xs py-0 px-1.5">
          {outputCount}{' '}
          {outputCount === 1 ? t('jsonParser.labels.variable') : t('jsonParser.labels.variables')}
        </Badge>
        {data.is_flatten_output ? (
          <Badge variant="outline" className="text-xs py-0 px-1.5">
            {t('jsonParser.labels.flatten')}
          </Badge>
        ) : null}
      </div>

      <OutputVariablesView variant="compact" variables={outputVariables} maxItems={2} />

      {data.error_strategy === 'fail-branch' ? (
        <div className="pt-1 border-t">
          <div className="relative h-[17px]">
            <div className="text-xs text-destructive font-medium text-right">
              {t('httpRequest.fields.failBranchLabel')}
            </div>
            <CustomHandle
              type="source"
              position={Position.Right}
              id="fail-branch"
              className="!bg-destructive"
              variant="destructive"
              style={{ top: '50%', right: -14 }}
            />
          </div>
        </div>
      ) : null}
    </div>
  );
};

export default JsonParserContent;
