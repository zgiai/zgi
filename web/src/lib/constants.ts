export const timezones = [
  {
    value: 'Pacific/Midway',
    offset: 'UTC-11:00',
    label: {
      'en-US': 'Samoa Standard Time',
      'zh-Hans': '萨摩亚标准时间',
    },
  },
  {
    value: 'Pacific/Honolulu',
    offset: 'UTC-10:00',
    label: {
      'en-US': 'Hawaii-Aleutian Standard Time',
      'zh-Hans': '夏威夷-阿留申标准时间',
    },
  },
  {
    value: 'America/Anchorage',
    offset: 'UTC-09:00',
    label: {
      'en-US': 'Alaska Time',
      'zh-Hans': '阿拉斯加时间',
    },
  },
  {
    value: 'America/Los_Angeles',
    offset: 'UTC-08:00',
    label: {
      'en-US': 'Pacific Time',
      'zh-Hans': '北美太平洋时间',
    },
  },
  {
    value: 'America/Denver',
    offset: 'UTC-07:00',
    label: {
      'en-US': 'Mountain Time',
      'zh-Hans': '北美山区时间',
    },
  },
  {
    value: 'America/Chicago',
    offset: 'UTC-06:00',
    label: {
      'en-US': 'Central Time',
      'zh-Hans': '北美中部时间',
    },
  },
  {
    value: 'America/New_York',
    offset: 'UTC-05:00',
    label: {
      'en-US': 'Eastern Time',
      'zh-Hans': '北美东部时间',
    },
  },
  {
    value: 'Atlantic/Bermuda',
    offset: 'UTC-04:00',
    label: {
      'en-US': 'Atlantic Time',
      'zh-Hans': '大西洋时间',
    },
  },
  {
    value: 'America/Sao_Paulo',
    offset: 'UTC-03:00',
    label: {
      'en-US': 'Brasilia Standard Time',
      'zh-Hans': '巴西利亚标准时间',
    },
  },
  {
    value: 'Atlantic/South_Georgia',
    offset: 'UTC-02:00',
    label: {
      'en-US': 'South Georgia Time',
      'zh-Hans': '南乔治亚岛时间',
    },
  },
  {
    value: 'Atlantic/Azores',
    offset: 'UTC-01:00',
    label: {
      'en-US': 'Azores Time',
      'zh-Hans': '亚速尔群岛时间',
    },
  },
  {
    value: 'Europe/London',
    offset: 'UTC+00:00',
    label: {
      'en-US': 'United Kingdom Time',
      'zh-Hans': '英国时间',
    },
  },
  {
    value: 'Europe/Paris',
    offset: 'UTC+01:00',
    label: {
      'en-US': 'Central European Time',
      'zh-Hans': '中欧时间',
    },
  },
  {
    value: 'Europe/Helsinki',
    offset: 'UTC+02:00',
    label: {
      'en-US': 'Eastern European Time',
      'zh-Hans': '东欧时间',
    },
  },
  {
    value: 'Europe/Moscow',
    offset: 'UTC+03:00',
    label: {
      'en-US': 'Moscow Standard Time',
      'zh-Hans': '莫斯科标准时间',
    },
  },
  {
    value: 'Asia/Dubai',
    offset: 'UTC+04:00',
    label: {
      'en-US': 'Gulf Standard Time',
      'zh-Hans': '海湾标准时间',
    },
  },
  {
    value: 'Asia/Karachi',
    offset: 'UTC+05:00',
    label: {
      'en-US': 'Pakistan Standard Time',
      'zh-Hans': '巴基斯坦标准时间',
    },
  },
  {
    value: 'Asia/Dhaka',
    offset: 'UTC+06:00',
    label: {
      'en-US': 'Bangladesh Standard Time',
      'zh-Hans': '孟加拉标准时间',
    },
  },
  {
    value: 'Asia/Bangkok',
    offset: 'UTC+07:00',
    label: {
      'en-US': 'Indochina Time',
      'zh-Hans': '中南半岛时间',
    },
  },
  {
    value: 'Asia/Hong_Kong',
    offset: 'UTC+08:00',
    label: {
      'en-US': 'Hong Kong Standard Time',
      'zh-Hans': '香港标准时间',
    },
  },
  {
    value: 'Asia/Shanghai',
    offset: 'UTC+08:00',
    label: {
      'en-US': 'China Standard Time',
      'zh-Hans': '中国标准时间',
    },
  },
  {
    value: 'Asia/Tokyo',
    offset: 'UTC+09:00',
    label: {
      'en-US': 'Japan Standard Time',
      'zh-Hans': '日本标准时间',
    },
  },
  {
    value: 'Australia/Sydney',
    offset: 'UTC+10:00',
    label: {
      'en-US': 'Australian Eastern Time',
      'zh-Hans': '澳大利亚东部时间',
    },
  },
  {
    value: 'Pacific/Auckland',
    offset: 'UTC+12:00',
    label: {
      'en-US': 'New Zealand Time',
      'zh-Hans': '新西兰时间',
    },
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
export type TimezoneValue = string;
export type LanguageValue = (typeof LANGUAGES)[number]['value'];
export type ThemeValue = (typeof THEMES)[number]['value'];
