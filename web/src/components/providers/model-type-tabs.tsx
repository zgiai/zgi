'use client';

import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import type { ModelUseCase } from '@/services/types/model';

interface ModelTypeTabsProps {
  availableTypes: Set<ModelUseCase>;
  selectedType: ModelUseCase | 'all';
  onChange: (val: ModelUseCase | 'all') => void;
  t: (key: string) => string;
}

const ORDER: ModelUseCase[] = [
  'text-chat',
  'vision',
  'image-gen',
  'embedding',
  'rerank',
  'speech-to-text',
  'text-to-speech',
  'realtime-audio',
  'video-gen',
  'moderation',
  'reasoning',
  'function-calling',
  'agent',
];

export default function ModelTypeTabs({
  availableTypes,
  selectedType,
  onChange,
  t,
}: ModelTypeTabsProps): JSX.Element {
  return (
    <Tabs value={selectedType} onValueChange={val => onChange(val as ModelUseCase | 'all')}>
      <TabsList>
        <TabsTrigger value="all">{t('models.filters.allTypes')}</TabsTrigger>
        {ORDER.filter(type => availableTypes.has(type)).map(type => (
          <TabsTrigger key={type} value={type}>
            {t(`usecases.${type}`)}
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  );
}
