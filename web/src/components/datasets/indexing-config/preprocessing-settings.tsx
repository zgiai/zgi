'use client';

import React, {
  useState,
  useEffect,
  useRef,
  useCallback,
  forwardRef,
  useImperativeHandle,
} from 'react';
import { useT } from '@/i18n';
import { Label } from '@/components/ui/label';
// import { Switch } from '@/components/ui/switch';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { type ProcessConfiguration } from '@/services/types/dataset';
import { Checkbox } from '@/components/ui/checkbox';
import { cn } from '@/lib/utils';

interface PreprocessingSettingsProps {
  processConfig: ProcessConfiguration | null;
  onChange: (config: ProcessConfiguration) => void;
  docLanguage?: string;
  onDocLanguageChange?: (language: string) => void;
  disableQAMode?: boolean;
  className?: string;
  ruleColumns?: number;
}

export interface PreprocessingSettingsRef {
  getFormData: () => {
    pre_processing_rules: Array<{ id: string; enabled: boolean }>;
    doc_language: string;
  };
}

const DEFAULT_RULES = [
  { id: 'remove_extra_spaces', enabled: false },
  { id: 'remove_urls_emails', enabled: false },
  { id: 'image_content_recognition', enabled: false },
  { id: 'segment_content_auto_fill', enabled: false },
  { id: 'formula_accuracy_enhance', enabled: false },
  { id: 'generate_recommend_questions', enabled: false },
];

export const PreprocessingSettings = forwardRef<
  PreprocessingSettingsRef,
  PreprocessingSettingsProps
>(
  (
    {
      processConfig,
      onChange,
      docLanguage = 'English',
      onDocLanguageChange,
      className,
      ruleColumns = 0,
    },
    ref
  ) => {
    const t = useT();
    const initializedRef = useRef(false);

    // Use API default rules if available, otherwise fall back to hardcoded defaults
    const getDefaultRules = () => {
      if (processConfig?.pre_processing_rules && processConfig.pre_processing_rules.length > 0) {
        return processConfig.pre_processing_rules;
      }
      return DEFAULT_RULES;
    };

    // Get the rules to render (from processConfig or defaults)
    const rulesToRender = getDefaultRules();

    const [rules, setRules] = useState(getDefaultRules);
    const [selectedLanguage, setSelectedLanguage] = useState(docLanguage);

    // Helper function to create updated config
    const createUpdatedConfig = useCallback(
      (updatedRules: typeof rules) => {
        return {
          pre_processing_rules: updatedRules,
          doc_language: selectedLanguage,
        };
      },
      [selectedLanguage]
    );

    // Expose getFormData method via ref
    useImperativeHandle(
      ref,
      () => ({
        getFormData: () => {
          return createUpdatedConfig(rules);
        },
      }),
      [rules, createUpdatedConfig]
    );

    // Only synchronize local state once on component initialization
    useEffect(() => {
      // On first mount, initialize local state
      if (!initializedRef.current) {
        const defaultRules = getDefaultRules();
        setRules(defaultRules);
        setSelectedLanguage(docLanguage);
        initializedRef.current = true;
        return;
      }

      // After initialization, keep local rules in sync if parent rules change
      const incomingRules = processConfig?.pre_processing_rules;
      if (Array.isArray(incomingRules) && incomingRules.length > 0) {
        const isDifferent =
          incomingRules.length !== rules.length ||
          incomingRules.some((r, i) => r.id !== rules[i]?.id || r.enabled !== rules[i]?.enabled);
        if (isDifferent) {
          setRules(incomingRules);
        }
      }

      // Sync language when parent props change
      setSelectedLanguage(docLanguage);
    }, [processConfig?.pre_processing_rules, docLanguage]);

    // Handle rules change and notify parent
    const handleRuleChange = useCallback(
      (ruleId: string, enabled: boolean) => {
        const updatedRules = rules.map(rule => (rule.id === ruleId ? { ...rule, enabled } : rule));
        setRules(updatedRules);

        // Notify parent immediately with updated config
        const updatedConfig = {
          pre_processing_rules: updatedRules,
          doc_language: selectedLanguage,
        };
        onChange?.({
          ...processConfig,
          ...updatedConfig,
          clean_mode: processConfig?.clean_mode ?? 'automatic',
          rules: processConfig?.rules ?? {},
        } as ProcessConfiguration);
      },
      [rules, selectedLanguage, onChange, processConfig]
    );

    const handleDocLanguageChange = useCallback(
      (language: string) => {
        setSelectedLanguage(language);

        // Notify parent with updated config
        const updatedConfig = {
          pre_processing_rules: rules,
          doc_language: language,
        };
        onChange?.({
          ...processConfig,
          ...updatedConfig,
          clean_mode: processConfig?.clean_mode ?? 'automatic',
          rules: processConfig?.rules ?? {},
        } as ProcessConfiguration);

        if (onDocLanguageChange) {
          onDocLanguageChange(language);
        }
      },
      [rules, onChange, processConfig, onDocLanguageChange]
    );

    return (
      <div className={className}>
        <div className="mb-1">
          <div className="flex items-center gap-2">
            <h3 className="text-lg font-semibold">
              {t('datasets.createWizard.processConfig.preprocessingRules')}
            </h3>
          </div>
        </div>
        <div className="space-y-4">
          {/* Pre-processing Rules */}
          <div
            className={cn(
              ruleColumns > 0 ? `grid grid-cols-${ruleColumns} gap-4` : 'flex flex-col gap-y-2'
            )}
          >
            {rulesToRender.map(rule => {
              const currentRule = rules.find(r => r.id === rule.id);
              return (
                <div key={rule.id} className="flex items-center justify-between">
                  <div className="space-x-1 flex items-center">
                    <Checkbox
                      checked={currentRule?.enabled || false}
                      onClick={() => handleRuleChange(rule.id, !currentRule?.enabled)}
                      id={rule.id}
                    />
                    <Label htmlFor={rule.id} className="text-sm font-medium cursor-pointer">
                      {t(
                        `datasets.createWizard.processConfig.preprocessing.rules.${rule.id}` as any
                      )}
                    </Label>
                  </div>
                </div>
              );
            })}
          </div>

          {/* Document Language Selection */}
          <div>
            <Label>{t('datasets.createWizard.processConfig.documentLanguage')}</Label>
            <Select value={selectedLanguage} onValueChange={handleDocLanguageChange}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="English">
                  {t('datasets.createWizard.processConfig.english')}
                </SelectItem>
                <SelectItem value="Chinese">
                  {t('datasets.createWizard.processConfig.chinese')}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
      </div>
    );
  }
);

PreprocessingSettings.displayName = 'PreprocessingSettings';
