import type { ModelItem } from '@/services/types/model';

const EXPERIENCE_ROUTES = [
  { useCase: 'video-gen', path: '/console/work/video' },
  { useCase: 'music-gen', path: '/console/work/music' },
  { useCase: 'image-gen', path: '/console/work/image' },
] as const;

export function getModelExperienceHref(model: ModelItem): string {
  const params = new URLSearchParams({
    provider: model.provider,
    model: model.model,
  });
  const route = EXPERIENCE_ROUTES.find(item => model.use_cases?.includes(item.useCase));

  // Chat is also the voice entry point: its microphone uses the workspace
  // speech-to-text default, while agent replies can use text-to-speech.
  return `${route?.path ?? '/console/work/chat'}?${params.toString()}`;
}
