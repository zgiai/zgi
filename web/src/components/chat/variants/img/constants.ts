/** AI image generation: model options */
export const IMAGE_MODELS = [
  { id: 'dall-e-3', name: 'DALL·E 3' },
  { id: 'dall-e-2', name: 'DALL·E 2' },
  { id: 'midjourney', name: 'Midjourney' },
  { id: 'flux', name: 'FLUX' },
] as const;

/** AI image generation: aspect ratio options */
export const IMAGE_ASPECT_RATIOS = [
  { id: '1:1', name: '1:1', labelKey: 'square' },
  { id: '3:4', name: '3:4', labelKey: 'portrait' },
  { id: '4:3', name: '4:3', labelKey: 'landscape' },
  { id: '16:9', name: '16:9', labelKey: 'widescreen' },
  { id: '9:16', name: '9:16', labelKey: 'vertical' },
] as const;

/** AI image generation: style options */
export const IMAGE_STYLES = [
  { id: 'business', name: '商务', label: '专业大气' },
  { id: 'minimal', name: '简约', label: '极简现代' },
  { id: 'ecommerce', name: '电商', label: '商品展示' },
  { id: '3d-render', name: '3D渲染', label: '立体效果' },
  { id: 'tech', name: '科技', label: '未来感' },
  { id: 'creative', name: '创意', label: '艺术风格' },
] as const;

/** AI image generation: number of images */
export const IMAGE_COUNTS = [
  { id: 1 },
  { id: 2 },
  { id: 3 },
  { id: 4 },
] as const;

/** AI image generation: resolution options */
export const IMAGE_RESOLUTIONS = [
  { id: '512', name: '512px', label: '快速' },
  { id: '1024', name: '1024px', label: '标准' },
  { id: '2048', name: '2048px', label: '高清' },
] as const;
