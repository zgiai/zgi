export const APP_AVATAR_PRESETS = [
  {
    id: 'little-z',
    labelKey: 'littleZ',
    src: '/images/app-avatars/little-z.webp',
    background: 'from-cyan-100 to-sky-200 dark:from-cyan-950 dark:to-sky-900',
  },
  {
    id: 'dolphin',
    labelKey: 'dolphin',
    src: '/images/app-avatars/dolphin.webp',
    background: 'from-sky-100 to-blue-200 dark:from-sky-950 dark:to-blue-900',
  },
  {
    id: 'octopus',
    labelKey: 'octopus',
    src: '/images/app-avatars/octopus.webp',
    background: 'from-violet-100 to-fuchsia-200 dark:from-violet-950 dark:to-fuchsia-900',
  },
  {
    id: 'jellyfish',
    labelKey: 'jellyfish',
    src: '/images/app-avatars/jellyfish.webp',
    background: 'from-rose-100 to-indigo-100 dark:from-rose-950 dark:to-indigo-950',
  },
  {
    id: 'whale',
    labelKey: 'whale',
    src: '/images/app-avatars/whale.webp',
    background: 'from-blue-100 to-slate-200 dark:from-blue-950 dark:to-slate-900',
  },
  {
    id: 'manta-ray',
    labelKey: 'mantaRay',
    src: '/images/app-avatars/manta-ray.webp',
    background: 'from-indigo-100 to-cyan-200 dark:from-indigo-950 dark:to-cyan-900',
  },
] as const;

export type AppAvatarPreset = (typeof APP_AVATAR_PRESETS)[number];

function hashString(value: string): number {
  let hash = 0;
  for (const char of value) {
    hash = (hash * 31 + char.charCodeAt(0)) >>> 0;
  }
  return hash;
}

export function getDefaultAppAvatar(appId: string): AppAvatarPreset {
  return APP_AVATAR_PRESETS[hashString(appId) % APP_AVATAR_PRESETS.length];
}

export function getRandomAppAvatar(): AppAvatarPreset {
  if (typeof crypto !== 'undefined' && typeof crypto.getRandomValues === 'function') {
    const randomValue = crypto.getRandomValues(new Uint32Array(1))[0];
    return APP_AVATAR_PRESETS[randomValue % APP_AVATAR_PRESETS.length];
  }

  return APP_AVATAR_PRESETS[Date.now() % APP_AVATAR_PRESETS.length];
}
