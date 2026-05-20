export const timezones = [
  {
    value: 'Pacific/Midway',
    label: '(GMT-11:00) Midway Island, Samoa',
  },
  {
    value: 'Pacific/Honolulu',
    label: '(GMT-10:00) Hawaii',
  },
  {
    value: 'America/Anchorage',
    label: '(GMT-09:00) Alaska',
  },
  {
    value: 'America/Los_Angeles',
    label: '(GMT-08:00) Pacific Time (US & Canada)',
  },
  {
    value: 'America/Denver',
    label: '(GMT-07:00) Mountain Time (US & Canada)',
  },
  {
    value: 'America/Chicago',
    label: '(GMT-06:00) Central Time (US & Canada)',
  },
  {
    value: 'America/New_York',
    label: '(GMT-05:00) Eastern Time (US & Canada)',
  },
  {
    value: 'Atlantic/Bermuda',
    label: '(GMT-04:00) Atlantic Time (Canada)',
  },
  {
    value: 'America/Sao_Paulo',
    label: '(GMT-03:00) Brasilia',
  },
  {
    value: 'Atlantic/South_Georgia',
    label: '(GMT-02:00) Mid-Atlantic',
  },
  {
    value: 'Atlantic/Azores',
    label: '(GMT-01:00) Azores',
  },
  {
    value: 'Europe/London',
    label: '(GMT+00:00) London, Edinburgh, Dublin',
  },
  {
    value: 'Europe/Paris',
    label: '(GMT+01:00) Paris, Berlin, Rome, Madrid',
  },
  {
    value: 'Europe/Helsinki',
    label: '(GMT+02:00) Helsinki, Athens, Istanbul',
  },
  {
    value: 'Europe/Moscow',
    label: '(GMT+03:00) Moscow, St. Petersburg',
  },
  {
    value: 'Asia/Dubai',
    label: '(GMT+04:00) Abu Dhabi, Dubai',
  },
  {
    value: 'Asia/Karachi',
    label: '(GMT+05:00) Islamabad, Karachi',
  },
  {
    value: 'Asia/Dhaka',
    label: '(GMT+06:00) Dhaka',
  },
  {
    value: 'Asia/Bangkok',
    label: '(GMT+07:00) Bangkok, Jakarta',
  },
  {
    value: 'Asia/Hong_Kong',
    label: '(GMT+08:00) Beijing, Hong Kong, Singapore',
  },
  {
    value: 'Asia/Shanghai',
    label: '(GMT+08:00) Shanghai, Taipei',
  },
  {
    value: 'Asia/Tokyo',
    label: '(GMT+09:00) Tokyo, Seoul',
  },
  {
    value: 'Australia/Sydney',
    label: '(GMT+10:00) Sydney, Melbourne',
  },
  {
    value: 'Pacific/Auckland',
    label: '(GMT+12:00) Auckland',
  },
] as const;

export const LANGUAGES = [
  {
    value: 'en-US',
    label: 'English (US)',
  },
  {
    value: 'zh-Hans',
    label: '中文 (简体)',
  },
] as const;

export const THEMES = [
  {
    value: 'light',
    label: 'Light',
  },
  {
    value: 'dark',
    label: 'Dark',
  },
  {
    value: 'blue',
    label: 'Ocean Blue',
  },
  {
    value: 'green',
    label: 'Nature Green',
  },
  {
    value: 'purple',
    label: 'Royal Purple',
  },
  {
    value: 'highContrast',
    label: 'High Contrast',
  },
] as const;

// Type exports for better TypeScript support
export type TimezoneValue = (typeof timezones)[number]['value'];
export type LanguageValue = (typeof LANGUAGES)[number]['value'];
export type ThemeValue = (typeof THEMES)[number]['value'];
