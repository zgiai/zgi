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

  // Speech models fall back to Chat until a dedicated speech workbench exists.
  return `${route?.path ?? '/console/work/chat'}?${params.toString()}`;
}
