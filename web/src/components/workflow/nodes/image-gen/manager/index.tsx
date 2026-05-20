'use client';

import React, { useCallback, useMemo, useRef } from 'react';
import { useT } from '@/i18n';
import { useNodeData, useNodeDataUpdate, useNodeOutputVariables } from '../../../hooks';
import type { ImageGenNodeData } from '../config';
import { cn } from '@/lib/utils';
import { WorkflowValueEditor, type WorkflowValueEditorHandle } from '@/components/workflow/ui';
import WorkflowValueInserter from '@/components/workflow/common/workflow-value-inserter';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import OutputVariablesView from '../../../common/output-variables-view';
import type { WorkflowVariable } from '../../../store/type';
import { ModelSelector } from '@/components/common/model-selector';

interface ImageGenManagerProps {
  id: string;
  className?: string;
  readOnly?: boolean;
}

const ImageGenManager: React.FC<ImageGenManagerProps> = ({
  id: nodeId,
  className,
  readOnly = false,
}) => {
  const t = useT();
  const updateData = useNodeDataUpdate<ImageGenNodeData>(nodeId);
  const selfNodeData = useNodeData<ImageGenNodeData>(nodeId);

  const editorRef = useRef<WorkflowValueEditorHandle>(null);

  const updateModel = useCallback(
    (patch: Partial<ImageGenNodeData['model']>) => {
      if (readOnly) return;
      updateData({
        model: {
          ...(selfNodeData?.model || { provider: '', name: '' }),
          ...patch,
        },
      });
    },
    [updateData, selfNodeData?.model, readOnly]
  );

  const handlePromptChange = useCallback(
    (value: string) => {
      if (readOnly) return;
      updateData({ prompt: value });
    },
    [updateData, readOnly]
  );

  const handleInsert = useCallback((value: any) => {
    if (readOnly) return;
    const { sourceId } = value;
    let key = value.key;
    // Normalize sys variable key
    if (sourceId === 'sys' && key.startsWith('sys.')) {
      key = key.slice(4);
    }
    editorRef.current?.insertToken(sourceId, key);
    editorRef.current?.focus();
  }, [readOnly]);

  const updateGeneration = useCallback(
    (patch: Partial<ImageGenNodeData['generation']>) => {
      if (readOnly) return;
      updateData({
        generation: {
          ...(selfNodeData?.generation || {
            n: 1,
            size: '512x512',
            quality: 'standard',
          }),
          ...patch,
        },
      });
    },
    [updateData, selfNodeData?.generation, readOnly]
  );


  const outputs = useNodeOutputVariables(nodeId);

  return (
    <div className={cn('space-y-6', className)}>
      {/* Model Selection */}
      <div className="space-y-4">
        <h3 className="text-base font-semibold">{t('nodes.imageGen.section.model')}</h3>
        <ModelSelector
          modelType="image-gen"
          value={{
            provider: selfNodeData?.model.provider || '',
            model: selfNodeData?.model.name || '',
          }}
          disabled={readOnly}
          onChange={v => {
            updateModel({
              provider: v.provider,
              name: v.model,
            });
          }}
        />
      </div>

      {/* Prompt */}
      <div className="space-y-4">
        <h3 className="text-base font-semibold">{t('nodes.imageGen.section.prompt')}</h3>
        <WorkflowValueInserter nodeId={nodeId} onInsert={handleInsert} disabled={readOnly} />
        <div className="border rounded-md p-2 min-h-[100px]">
          <WorkflowValueEditor
            ref={editorRef}
            value={selfNodeData?.prompt || ''}
            onChange={handlePromptChange}
            placeholder={t('nodes.imageGen.placeholders.prompt')}
            nodeId={nodeId}
            editorClassName="min-h-[100px]"
            readOnly={readOnly}
          />
        </div>
      </div>

      {/* Generation Config */}
      <div className="space-y-4">
        <h3 className="text-base font-semibold">{t('nodes.imageGen.section.generation')}</h3>

        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label>{t('nodes.imageGen.fields.size')}</Label>
            <Select
              value={selfNodeData?.generation.size || '1024x1024'}
              onValueChange={val => updateGeneration({ size: val })}
              disabled={readOnly}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="1024x1024">1024x1024 (1:1)</SelectItem>
                <SelectItem value="1024x768">1024x768 (4:3)</SelectItem>
                <SelectItem value="768x1024">768x1024 (3:4)</SelectItem>
                <SelectItem value="1920x1080">1920x1080 (16:9)</SelectItem>
                <SelectItem value="1080x1920">1080x1920 (9:16)</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <Label>{t('nodes.imageGen.fields.n')}</Label>
            <Input
              type="number"
              min={1}
              max={4}
              value={selfNodeData?.generation.n || 1}
              onChange={e => updateGeneration({ n: parseInt(e.target.value) || 1 })}
              disabled={readOnly}
            />
          </div>
        </div>

        <div className="space-y-2">
          <Label>{t('nodes.imageGen.fields.quality')}</Label>
          <Select
            value={selfNodeData?.generation.quality || 'standard'}
            onValueChange={val => updateGeneration({ quality: val })}
            disabled={readOnly}
          >
            <SelectTrigger>
              <SelectValue placeholder="Default" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="standard">Standard</SelectItem>
              <SelectItem value="hd">HD</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>
      {/* Output Config */}
      <div className="space-y-4">
        <h3 className="text-base font-semibold">{t('nodes.imageGen.section.output')}</h3>
        <div className="flex items-center justify-between">
          <Label>{t('nodes.imageGen.fields.lifecycle')}</Label>
          <Select
            value={selfNodeData?.output.lifecycle || 'persistent'}
            onValueChange={val =>
              updateData({
                output: { ...selfNodeData?.output, lifecycle: val as 'persistent' | 'temporary' },
              })
            }
            disabled={readOnly}
          >
            <SelectTrigger className="w-[180px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="persistent">{t('nodes.imageGen.options.persistent')}</SelectItem>
              <SelectItem value="temporary">{t('nodes.imageGen.options.temporary')}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      <OutputVariablesView variables={outputs} />
    </div>
  );
};

export default ImageGenManager;

