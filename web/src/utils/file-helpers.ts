/**
 * File utility functions
 */

// Shared file extension categories used across upload filtering and UI
export const IMAGE_EXTENSIONS: readonly string[] = ['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg'];

export const ORIGINAL_PREVIEW_IMAGE_EXTENSIONS: readonly string[] = [
  'jpg',
  'jpeg',
  'png',
  'webp',
  'gif',
  'svg',
];

export const ORIGINAL_PREVIEW_TEXT_EXTENSIONS: readonly string[] = [
  'txt',
  'md',
  'markdown',
  'mdx',
  'csv',
  'xml',
];

export const ORIGINAL_PREVIEW_EXTENSIONS: readonly string[] = [
  'pdf',
  ...ORIGINAL_PREVIEW_IMAGE_EXTENSIONS,
  ...ORIGINAL_PREVIEW_TEXT_EXTENSIONS,
];

export const AUDIO_EXTENSIONS: readonly string[] = ['mp3', 'm4a', 'wav', 'amr', 'mpga'];

export const VIDEO_EXTENSIONS: readonly string[] = ['mp4', 'mov', 'webm', 'mpeg'];

export const DOCUMENT_EXTENSIONS: readonly string[] = [
  'txt',
  'md',
  'mdx',
  'markdown',
  'pdf',
  'html',
  'xlsx',
  'xls',
  'doc',
  'docx',
  'csv',
  'eml',
  'msg',
  'pptx',
  'ppt',
  'xml',
  'epub',
];

// Mapping of file type categories to their extensions
export const FILE_TYPE_EXTENSIONS: Record<string, readonly string[]> = {
  image: IMAGE_EXTENSIONS,
  audio: AUDIO_EXTENSIONS,
  video: VIDEO_EXTENSIONS,
  document: DOCUMENT_EXTENSIONS,
};

/**
 * @util Check whether a file extension supports original file preview.
 */
export function isOriginalPreviewSupported(
  extension?: string | null,
  mimeType?: string | null
): boolean {
  if (mimeType?.toLowerCase().startsWith('image/')) return true;
  if (mimeType?.toLowerCase() === 'application/pdf') return true;
  if (!extension) return false;

  return ORIGINAL_PREVIEW_EXTENSIONS.includes(extension.toLowerCase().replace(/^\./, ''));
}

/**
 * @util Check whether an original preview should render as an image.
 */
export function isOriginalPreviewImage(
  extension?: string | null,
  mimeType?: string | null
): boolean {
  if (mimeType?.toLowerCase().startsWith('image/')) return true;
  if (!extension) return false;

  return ORIGINAL_PREVIEW_IMAGE_EXTENSIONS.includes(extension.toLowerCase().replace(/^\./, ''));
}

/**
 * @util Check whether an original preview should render as a PDF.
 */
export function isOriginalPreviewPdf(extension?: string | null, mimeType?: string | null): boolean {
  if (mimeType?.toLowerCase() === 'application/pdf') return true;
  if (!extension) return false;

  return extension.toLowerCase().replace(/^\./, '') === 'pdf';
}

/**
 * @util Check whether an original preview should render as browser text.
 */
export function isOriginalPreviewText(extension?: string | null): boolean {
  if (!extension) return false;

  return ORIGINAL_PREVIEW_TEXT_EXTENSIONS.includes(extension.toLowerCase().replace(/^\./, ''));
}

/**
 * Filters an array of file extensions to only include lowercase versions
 * Removes duplicates and normalizes extensions (removes leading dots)
 *
 * @param extensions - Array of file extensions (can be mixed case)
 * @returns Array of unique lowercase file extensions without leading dots
 *
 * @example
 * ```typescript
 * const mixedExtensions = ["txt", "PDF", "DOC", "docx", "TXT"];
 * const lowercaseExtensions = filterLowercaseExtensions(mixedExtensions);
 * // Result: ["txt", "pdf", "doc", "docx"]
 * ```
 */
export function filterLowercaseExtensions(extensions: string[]): string[] {
  const normalizedExtensions = extensions.map(ext => {
    // Remove leading dot if present and convert to lowercase
    const normalized = ext.toLowerCase().replace(/^\./, '');
    return normalized;
  });

  // Remove duplicates using Set
  return [...new Set(normalizedExtensions)];
}

/**
 * Formats file extensions for display (adds leading dots)
 *
 * @param extensions - Array of file extensions without leading dots
 * @returns Array of file extensions with leading dots
 *
 * @example
 * ```typescript
 * const extensions = ["txt", "pdf", "doc"];
 * const formattedExtensions = formatExtensionsForDisplay(extensions);
 * // Result: [".txt", ".pdf", ".doc"]
 * ```
 */
export function formatExtensionsForDisplay(extensions: string[]): string[] {
  return filterLowercaseExtensions(extensions).map(ext => `.${ext}`);
}

/**
 * Converts file type categories to their corresponding extensions.
 * Handles 'custom' type by returning empty array (custom extensions handled separately).
 *
 * @param fileTypes - Array of file type categories (e.g., ['image', 'document'])
 * @returns Array of unique file extensions for the given types
 *
 * @example
 * ```typescript
 * const types = ['image', 'document'];
 * const extensions = getExtensionsFromFileTypes(types);
 * // Result: ['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg', 'txt', 'md', ...]
 * ```
 */
export function getExtensionsFromFileTypes(fileTypes: string[]): string[] {
  const extensions: string[] = [];
  for (const type of fileTypes) {
    // Skip 'custom' type - custom extensions are handled separately
    if (type === 'custom') continue;
    const typeExtensions = FILE_TYPE_EXTENSIONS[type];
    if (typeExtensions) {
      extensions.push(...typeExtensions);
    }
  }
  // Remove duplicates
  return [...new Set(extensions)];
}

/**
 * @util Resolve the effective allowed extensions for a workflow file input.
 *
 * `custom` is treated as an exclusive mode. When selected, only explicit custom
 * extensions are honored. Otherwise, only category-derived extensions are used.
 */
export function getEffectiveAllowedFileExtensions(
  fileTypes: string[] = [],
  allowedExtensions: string[] = []
): string[] {
  const normalizedTypes = Array.isArray(fileTypes) ? fileTypes : [];
  if (normalizedTypes.includes('custom')) {
    return filterLowercaseExtensions(allowedExtensions);
  }

  return getExtensionsFromFileTypes(normalizedTypes);
}

/**
 * @util Resolve the effective allowed extensions for chat-style workflow uploads.
 *
 * The result follows workflow feature semantics:
 * - `custom` is exclusive and only honors explicit custom extensions
 * - non-`custom` uses type-derived extensions
 * - `document` is derived from supported server extensions minus image/audio/video
 * - final output is normalized and intersected with server-supported extensions when provided
 */
export function getEffectiveChatUploadExtensions(
  fileTypes: string[] = [],
  allowedExtensions: string[] = [],
  supportedExtensions: string[] = []
): string[] {
  const normalizedTypes = Array.isArray(fileTypes) ? fileTypes : [];
  const normalizedSupported = filterLowercaseExtensions(supportedExtensions);
  const hasSupportedExtensions = normalizedSupported.length > 0;

  if (normalizedTypes.includes('custom')) {
    const normalizedCustomExtensions = filterLowercaseExtensions(allowedExtensions);
    if (!hasSupportedExtensions) return normalizedCustomExtensions;

    return normalizedCustomExtensions.filter(ext => normalizedSupported.includes(ext));
  }

  const typeSet = new Set(normalizedTypes);
  const effectiveExtensions: string[] = [];

  if (typeSet.has('image')) effectiveExtensions.push(...IMAGE_EXTENSIONS);
  if (typeSet.has('audio')) effectiveExtensions.push(...AUDIO_EXTENSIONS);
  if (typeSet.has('video')) effectiveExtensions.push(...VIDEO_EXTENSIONS);
  if (typeSet.has('document')) {
    const documentExtensions = hasSupportedExtensions
      ? normalizedSupported.filter(
          ext =>
            !IMAGE_EXTENSIONS.includes(ext) &&
            !AUDIO_EXTENSIONS.includes(ext) &&
            !VIDEO_EXTENSIONS.includes(ext)
        )
      : [...DOCUMENT_EXTENSIONS];
    effectiveExtensions.push(...documentExtensions);
  }

  const normalizedEffectiveExtensions = filterLowercaseExtensions(effectiveExtensions);
  if (!hasSupportedExtensions) return normalizedEffectiveExtensions;

  return normalizedEffectiveExtensions.filter(ext => normalizedSupported.includes(ext));
}

/**
 * @util Check whether an extension is allowed by a normalized extension whitelist.
 */
export function isAllowedUploadExtension(
  extension: string | null | undefined,
  allowedExtensions: string[] = []
): boolean {
  const normalizedExtension = extension?.toLowerCase().replace(/^\./, '') ?? '';
  if (!normalizedExtension) return false;

  return filterLowercaseExtensions(allowedExtensions).includes(normalizedExtension);
}

/**
 * @util Build a native file input `accept` attribute from extensions.
 */
export function buildFileInputAcceptAttribute(extensions: string[]): string | undefined {
  const normalized = filterLowercaseExtensions(extensions);
  if (normalized.length === 0) return undefined;

  return normalized.map(ext => `.${ext}`).join(',');
}

/**
 * Validates if a file extension is in the allowed list
 *
 * @param fileName - The file name to check
 * @param allowedExtensions - Array of allowed extensions (case-insensitive)
 * @returns True if the file extension is allowed
 */
export function isValidFileExtension(fileName: string, allowedExtensions: string[]): boolean {
  const fileExt = fileName.split('.').pop()?.toLowerCase();
  if (!fileExt) return false;

  const normalizedAllowedExtensions = allowedExtensions.map(ext =>
    ext.toLowerCase().replace(/^\./, '')
  );

  return normalizedAllowedExtensions.includes(fileExt);
}

/* -------------------------------------------------------------------------- */
/* Image file size helpers                                                     */
/* -------------------------------------------------------------------------- */

/**
 * Validate image file size is within the specified max bytes.
 */
export function isImageFileWithinSize(file: File, maxBytes: number): boolean {
  return file.size <= maxBytes;
}
