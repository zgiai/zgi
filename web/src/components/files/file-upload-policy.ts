export const FILE_MANAGEMENT_UPLOAD_ACCEPT_EXT = [
  'pdf',
  'docx',
  'doc',
  'xlsx',
  'xls',
  'csv',
  'txt',
  'md',
  'markdown',
  'mdx',
  'png',
  'jpg',
  'jpeg',
  'pptx',
  'ppt',
] as const;

export type FileManagementUploadAcceptExt = (typeof FILE_MANAGEMENT_UPLOAD_ACCEPT_EXT)[number];
