import React, { useState, useEffect, useRef, forwardRef, useImperativeHandle } from 'react';
import { useT } from '@/i18n';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Info, Search } from 'lucide-react';
import type { InternalRetrievalConfig, SearchMethod } from '@/services/types/dataset';
import { Slider } from '@/components/ui/slider';
import { useInitializeDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { ModelSelector } from '@/components/common/model-selector';
import { normalizeDatasetSearchMethod } from '@/utils/dataset/retrieval-config';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';

// Re-export the unified type for backward compatibility
export type RetrievalConfig = InternalRetrievalConfig;

interface RetrievalSettingsProps {
  retrieval: RetrievalConfig;
  disabled?: boolean;
  isGraphEnabled?: boolean;
  onChange?: (retrieval: RetrievalConfig) => void;
}

export interface RetrievalSettingsRef {
  getFormData: () => RetrievalConfig;
}

// Default values for initialization
const DEFAULT_RETRIEVAL_CONFIG: RetrievalConfig = {
  search_method: 'semantic_search',
  top_k: 3,
  score_threshold_enabled: true,
  score_threshold: 0.5,
  reranking_enable: true,
  reranking_model: {
    reranking_provider_name: '',
    reranking_model_name: '',
  },
};

function SettingLabelWithTooltip({
  htmlFor,
  label,
  tooltip,
}: {
  htmlFor?: string;
  label: string;
  tooltip: string;
}) {
  return (
    <div className="flex items-center gap-1.5">
      <Label htmlFor={htmlFor} className="text-sm font-medium">
        {label}
      </Label>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            className="inline-flex h-4 w-4 items-center justify-center rounded-full text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            aria-label={tooltip}
          >
            <Info className="h-3.5 w-3.5" />
          </button>
        </TooltipTrigger>
        <TooltipContent side="top" align="start" className="max-w-80 text-sm leading-6">
          {tooltip}
        </TooltipContent>
      </Tooltip>
    </div>
  );
}

export const RetrievalSettings = forwardRef<RetrievalSettingsRef, RetrievalSettingsProps>(
  ({ retrieval, disabled = false, isGraphEnabled = false, onChange }, ref) => {
    const t = useT('datasets');
    const initializedRef = useRef(false);
    // Track if the latest local state change was user-initiated to avoid notifying on initial sync
    const userChangeRef = useRef(false);

    // Local state management
    const [localRetrieval, setLocalRetrieval] = useState<RetrievalConfig>(() => {
      const initialConfig: RetrievalConfig = {
        search_method: normalizeDatasetSearchMethod(retrieval.search_method, isGraphEnabled),
        top_k: retrieval.top_k ?? DEFAULT_RETRIEVAL_CONFIG.top_k,
        score_threshold_enabled:
          retrieval.score_threshold_enabled ?? DEFAULT_RETRIEVAL_CONFIG.score_threshold_enabled,
        score_threshold: retrieval.score_threshold ?? DEFAULT_RETRIEVAL_CONFIG.score_threshold,
        reranking_enable: retrieval.reranking_enable ?? DEFAULT_RETRIEVAL_CONFIG.reranking_enable,
        reranking_model: retrieval.reranking_model ?? DEFAULT_RETRIEVAL_CONFIG.reranking_model,
      };

      return initialConfig;
    });

    const [topKInput, setTopKInput] = useState<string>(String(localRetrieval.top_k));

    // Synchronize local state when retrieval prop changes
    useEffect(() => {
      const newTopK = retrieval.top_k ?? DEFAULT_RETRIEVAL_CONFIG.top_k;
      setLocalRetrieval({
        search_method: normalizeDatasetSearchMethod(retrieval.search_method, isGraphEnabled),
        top_k: newTopK,
        score_threshold_enabled:
          retrieval.score_threshold_enabled ?? DEFAULT_RETRIEVAL_CONFIG.score_threshold_enabled,
        score_threshold: retrieval.score_threshold ?? DEFAULT_RETRIEVAL_CONFIG.score_threshold,
        reranking_enable: retrieval.reranking_enable ?? DEFAULT_RETRIEVAL_CONFIG.reranking_enable,
        reranking_model: retrieval.reranking_model ?? DEFAULT_RETRIEVAL_CONFIG.reranking_model,
      });
      setTopKInput(String(newTopK));

      initializedRef.current = true;
    }, [retrieval, isGraphEnabled]);

    // Prefill default rerank model when hooks resolve and current is empty
    useInitializeDefaultModelByUseCase({
      useCase: 'rerank',
      currentModel: {
        provider: localRetrieval.reranking_model?.reranking_provider_name,
        model: localRetrieval.reranking_model?.reranking_model_name,
      },
      enabled: Boolean(localRetrieval.reranking_enable),
      onInitialize: v => {
        setLocalRetrieval(prev => ({
          ...prev,
          reranking_model: {
            reranking_provider_name: v.provider,
            reranking_model_name: v.model,
          },
        }));
      },
    });

    // Expose getFormData method via ref
    useImperativeHandle(
      ref,
      () => ({
        getFormData: () => ({
          ...localRetrieval,
        }),
      }),
      [localRetrieval]
    );

    // Local update function (user-initiated changes)
    const updateLocalRetrieval = <K extends keyof RetrievalConfig>(
      key: K,
      value: RetrievalConfig[K]
    ) => {
      // Mark this update as user-initiated to notify parent in effect after commit
      userChangeRef.current = true;
      setLocalRetrieval(prev => ({
        ...prev,
        [key]: value,
      }));
      if (key === 'top_k') {
        setTopKInput(String(value));
      }
    };

    const handleTopKBlur = () => {
      const val = parseInt(topKInput, 10);
      if (isNaN(val) || val < 1 || val > 10) {
        // Rollback to last valid state
        setTopKInput(String(localRetrieval.top_k));
      } else {
        updateLocalRetrieval('top_k', val);
      }
    };

    // Notify parent after local state commits to avoid setState during render in another component
    useEffect(() => {
      if (onChange && initializedRef.current && userChangeRef.current) {
        onChange(localRetrieval);
        // Reset flag after notifying parent
        userChangeRef.current = false;
      }
    }, [localRetrieval, onChange]);

    return (
      <TooltipProvider>
        <div>
          <div className="mb-4">
            <div className="flex items-center gap-2">
              <Search className="h-4 w-4" />
              <h3 className="text-lg font-semibold">
                {t('createWizard.processConfig.retrievalConfig')}
              </h3>
            </div>
          </div>
          <div className="space-y-6">
            {/* Search Method Selection - always visible */}
            <div className="space-y-2">
              <Label className="leading-5">{t('createWizard.processConfig.searchMethod')}</Label>
              <Select
                value={localRetrieval.search_method}
                onValueChange={val => updateLocalRetrieval('search_method', val as SearchMethod)}
                disabled={disabled}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="semantic_search">
                    {t('hitTesting.methods.semantic_search')}
                  </SelectItem>
                  {isGraphEnabled && (
                    <SelectItem value="graph_search">
                      {t('hitTesting.methods.graph_search')}
                    </SelectItem>
                  )}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-6 w-full">
              <div className="space-y-3 w-full">
                <SettingLabelWithTooltip
                  htmlFor="top_k"
                  label={t('createWizard.processConfig.topK')}
                  tooltip={t('createWizard.processConfig.topKHelp')}
                />
                <div className="flex items-center gap-6 w-full">
                  <Slider
                    min={1}
                    max={10}
                    step={1}
                    value={[localRetrieval.top_k]}
                    onValueChange={([val]) => updateLocalRetrieval('top_k', val)}
                    disabled={disabled}
                    className="flex-1"
                  />
                  <Input
                    id="top_k"
                    className="w-20 h-9 text-center"
                    value={topKInput}
                    root
                    onChange={e => setTopKInput(e.target.value)}
                    onBlur={handleTopKBlur}
                    disabled={disabled}
                  />
                </div>
              </div>

              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <SettingLabelWithTooltip
                    htmlFor="score_threshold"
                    label={t('createWizard.processConfig.scoreThreshold')}
                    tooltip={t('createWizard.processConfig.scoreThresholdHelp')}
                  />
                  <Switch
                    checked={localRetrieval.score_threshold_enabled}
                    onCheckedChange={checked =>
                      updateLocalRetrieval('score_threshold_enabled', checked)
                    }
                    disabled={disabled}
                  />
                </div>

                <div className="flex items-center gap-6">
                  <Slider
                    min={0}
                    max={1}
                    step={0.05}
                    value={[localRetrieval.score_threshold]}
                    onValueChange={([val]) => updateLocalRetrieval('score_threshold', val)}
                    disabled={disabled || !localRetrieval.score_threshold_enabled}
                    className="flex-1"
                  />
                  <Input
                    id="score_threshold"
                    type="number"
                    step={0.05}
                    min={0}
                    max={1}
                    root
                    className="w-20 h-9 text-center font-mono"
                    disabled={disabled || !localRetrieval.score_threshold_enabled}
                    value={localRetrieval.score_threshold}
                    onChange={e => updateLocalRetrieval('score_threshold', Number(e.target.value))}
                  />
                </div>
              </div>
            </div>
            <div className="space-y-2 pb-4">
              <div className="flex items-center justify-between">
                <SettingLabelWithTooltip
                  label={t('createWizard.processConfig.enableReranking')}
                  tooltip={t('createWizard.processConfig.rerankingHelp')}
                />
                <Switch
                  checked={localRetrieval.reranking_enable}
                  onCheckedChange={checked => {
                    // Toggle reranking and set mode for model selector in a user-initiated batch
                    updateLocalRetrieval('reranking_enable', checked);
                    // When enabling rerank, prefill default rerank model if empty
                    if (checked) {
                      updateLocalRetrieval('reranking_model', localRetrieval.reranking_model);
                    }
                  }}
                  disabled={disabled}
                />
              </div>
              {/* Show model selector when reranking is enabled for non-hybrid search */}
              {localRetrieval.reranking_enable && (
                <div className="space-y-2">
                  <ModelSelector
                    modelType="rerank"
                    value={{
                      provider: localRetrieval.reranking_model?.reranking_provider_name || '',
                      model: localRetrieval.reranking_model?.reranking_model_name || '',
                    }}
                    onChange={({ provider, model }) =>
                      updateLocalRetrieval('reranking_model', {
                        reranking_provider_name: provider,
                        reranking_model_name: model,
                      })
                    }
                    className="max-w-md"
                    disabled={disabled}
                  />
                </div>
              )}
            </div>
          </div>
        </div>
      </TooltipProvider>
    );
  }
);

RetrievalSettings.displayName = 'RetrievalSettings';
