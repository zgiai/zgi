import type { ModelItem } from '@/services/types/model';

type ChannelModelMetadata = Pick<ModelItem, 'endpoints' | 'use_cases'>;

export function isDoubaoSpeechModel(model: ChannelModelMetadata): boolean {
  const useCases = model.use_cases ?? [];
  return (
    useCases.includes('text-to-speech') ||
    useCases.includes('speech-to-text') ||
    model.endpoints.speech_generation === true ||
    model.endpoints.transcription === true
  );
}
