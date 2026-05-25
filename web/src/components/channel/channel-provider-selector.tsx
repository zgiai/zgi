'use client';

import React from 'react';
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useT } from '@/i18n';
import { ProviderIcon } from '@/components/common/provider-icon';

export interface ChannelProviderOption {
  value: string;
  labelKey: string;
  icon?: string;
  provider: string;
  category: 'aggregator' | 'compatible' | 'vertical';
  defaultApiBaseUrl?: string;
  apiBaseUrlPlaceholder?: string;
  apiKeyPlaceholder?: string;
  notesKey?: string;
}

export const CHANNEL_PROVIDER_OPTIONS: ChannelProviderOption[] = [
  {
    value: 'openai-compatible',
    labelKey: 'dialog.protocolOptions.openaiCompatible',
    provider: 'all',
    category: 'compatible',
    defaultApiBaseUrl: 'https://api.openai.com/v1',
    apiKeyPlaceholder: 'sk-xxx',
  },
  {
    value: 'ollama',
    labelKey: 'dialog.protocolOptions.ollama',
    icon: 'ollama',
    provider: 'all',
    category: 'vertical',
    defaultApiBaseUrl: 'http://localhost:11434',
    apiKeyPlaceholder: 'ollama',
  },
  {
    value: 'openai',
    labelKey: 'dialog.protocolOptions.openai',
    icon: 'openai',
    provider: 'openai',
    category: 'vertical',
    defaultApiBaseUrl: 'https://api.openai.com/v1',
    apiKeyPlaceholder: 'sk-xxx',
  },
  {
    value: 'glm',
    labelKey: 'dialog.protocolOptions.glm',
    icon: 'zhipu',
    provider: 'zhipu',
    category: 'vertical',
    defaultApiBaseUrl: 'https://open.bigmodel.cn/api/paas/v4',
    apiKeyPlaceholder: 'xxx',
  },
  {
    value: 'minimax',
    labelKey: 'dialog.protocolOptions.minimax',
    icon: 'minimax',
    provider: 'minimax',
    category: 'vertical',
    defaultApiBaseUrl: 'https://api.minimaxi.com/v1',
    apiKeyPlaceholder: 'xxx',
  },
  {
    value: 'deepseek',
    labelKey: 'dialog.protocolOptions.deepseek',
    icon: 'deepseek',
    provider: 'deepseek',
    category: 'vertical',
    defaultApiBaseUrl: 'https://api.deepseek.com/v1',
    apiKeyPlaceholder: 'sk-xxx',
  },
  {
    value: 'mistral',
    labelKey: 'dialog.protocolOptions.mistral',
    icon: 'mistral',
    provider: 'mistral',
    category: 'vertical',
    defaultApiBaseUrl: 'https://api.mistral.ai/v1',
    apiKeyPlaceholder: 'xxx',
  },
  {
    value: 'cohere',
    labelKey: 'dialog.protocolOptions.cohere',
    icon: 'cohere',
    provider: 'cohere',
    category: 'vertical',
    defaultApiBaseUrl: 'https://api.cohere.com',
    apiKeyPlaceholder: 'xxx',
  },
  {
    value: 'openrouter',
    labelKey: 'dialog.protocolOptions.openrouter',
    icon: 'openrouter',
    provider: 'openrouter',
    category: 'aggregator',
    defaultApiBaseUrl: 'https://openrouter.ai/api/v1',
    apiKeyPlaceholder: 'sk-or-xxx',
  },
  {
    value: 'anthropic',
    labelKey: 'dialog.protocolOptions.anthropic',
    icon: 'anthropic',
    provider: 'anthropic',
    category: 'vertical',
    defaultApiBaseUrl: 'https://api.anthropic.com/v1',
    apiKeyPlaceholder: 'sk-ant-xxx',
  },
  {
    value: 'qwen',
    labelKey: 'dialog.protocolOptions.qwen',
    icon: 'qwen',
    provider: 'qwen',
    category: 'vertical',
    defaultApiBaseUrl: 'https://dashscope.aliyuncs.com/api/v1',
    apiKeyPlaceholder: 'sk-xxx',
  },
  {
    value: 'moonshotai-cn',
    labelKey: 'dialog.protocolOptions.moonshotaiCn',
    icon: 'moonshot',
    provider: 'moonshot',
    category: 'vertical',
    defaultApiBaseUrl: 'https://api.moonshot.cn/v1',
    apiKeyPlaceholder: 'sk-xxx',
  },
  {
    value: 'doubao',
    labelKey: 'dialog.protocolOptions.doubao',
    icon: 'doubao',
    provider: 'doubao',
    category: 'vertical',
    defaultApiBaseUrl: 'https://ark.cn-beijing.volces.com/api/v3',
    apiKeyPlaceholder: 'sk-xxx',
  },
  {
    value: 'google',
    labelKey: 'dialog.protocolOptions.google',
    icon: 'google',
    provider: 'google',
    category: 'vertical',
    defaultApiBaseUrl: 'https://generativelanguage.googleapis.com/v1beta',
    apiKeyPlaceholder: 'Gemini API Key 或 Vertex Bearer Token',
    notesKey: 'dialog.protocolNotes.google',
  },
];

interface ChannelProviderSelectorProps {
  value: string;
  onChange: (provider: ChannelProviderOption) => void;
  disabled?: boolean;
}

export function getChannelProviderOption(value?: string): ChannelProviderOption | undefined {
  if (!value) return undefined;
  const normalized = value.trim().toLowerCase();
  return CHANNEL_PROVIDER_OPTIONS.find(
    item =>
      item.value.toLowerCase() === normalized ||
      item.provider?.toLowerCase() === normalized ||
      item.icon?.toLowerCase() === normalized
  );
}

export function getMappedProvider(value?: string): string {
  if (!value) return '';
  return getChannelProviderOption(value)?.provider ?? value.trim().toLowerCase();
}

const PROVIDER_GROUPS: Array<{
  category: ChannelProviderOption['category'];
  labelKey: string;
}> = [
  { category: 'aggregator', labelKey: 'dialog.protocolGroups.aggregator' },
  { category: 'compatible', labelKey: 'dialog.protocolGroups.compatible' },
  { category: 'vertical', labelKey: 'dialog.protocolGroups.vertical' },
];

export default function ChannelProviderSelector({
  value,
  onChange,
  disabled,
}: ChannelProviderSelectorProps): JSX.Element {
  const t = useT('channels');
  const selectedOption = getChannelProviderOption(value);

  return (
    <Select
      value={value}
      onValueChange={next => {
        const option = getChannelProviderOption(next);
        if (option) onChange(option);
      }}
      disabled={disabled}
    >
      <SelectTrigger>
        <SelectValue placeholder={t('dialog.placeholders.provider')}>
          {selectedOption ? (
            <div className="flex min-w-0 items-center gap-2">
              {selectedOption.icon ? (
                <ProviderIcon size={18} provider={selectedOption.icon} />
              ) : null}
              <span className="truncate">{t(selectedOption.labelKey as never)}</span>
            </div>
          ) : undefined}
        </SelectValue>
      </SelectTrigger>
      <SelectContent>
        {PROVIDER_GROUPS.map((group, index) => {
          const options = CHANNEL_PROVIDER_OPTIONS.filter(
            option => option.category === group.category
          );
          if (options.length === 0) return null;
          return (
            <React.Fragment key={group.category}>
              {index > 0 && <SelectSeparator />}
              <SelectGroup>
                <SelectLabel className="px-2 py-1.5 text-xs font-semibold text-muted-foreground">
                  {t(group.labelKey as never)}
                </SelectLabel>
                {options.map(option => (
                  <SelectItem key={option.value} value={option.value}>
                    <div className="flex items-center gap-2">
                      {option.icon ? <ProviderIcon size={20} provider={option.icon} /> : null}
                      {t(option.labelKey as never)}
                    </div>
                  </SelectItem>
                ))}
              </SelectGroup>
            </React.Fragment>
          );
        })}
      </SelectContent>
    </Select>
  );
}
