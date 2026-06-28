import {
  FILE_FOLDERS_KEY,
  FILES_QUERY_KEY,
  STORAGE_USAGE_KEY,
} from '@/hooks/use-files';
import { FILE_KEYS } from '@/hooks/query-keys';
import type { FileItem } from '@/services/types/file';
import type { AIChatCapabilityDescriptor } from '@/components/aichat/page-context';
import type { FilesAIChatContextItem, FilesAIChatContextSnapshot } from './types';

const FILES_CONTEXT_VISIBLE_LIMIT = 20;
const FILES_PAGE_SORT_KEY = 'created_at';
const FILES_PAGE_SORT_DIRECTION = 'desc';
const AI_CHAT_EXCEL_EXTENSIONS = new Set(['xls', 'xlsx', 'xlsm', 'xlsb']);
const AI_CHAT_WORD_EXTENSIONS = new Set(['doc', 'docx']);
const AI_CHAT_PRESENTATION_EXTENSIONS = new Set(['ppt', 'pptx']);
const AI_CHAT_IMAGE_EXTENSIONS = new Set(['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg']);
const AI_CHAT_AUDIO_EXTENSIONS = new Set(['mp3', 'm4a', 'wav', 'amr', 'mpga']);
const AI_CHAT_VIDEO_EXTENSIONS = new Set(['mp4', 'mov', 'webm', 'mpeg']);
const AI_CHAT_TEXT_EXTENSIONS = new Set(['txt', 'md', 'markdown', 'mdx', 'json', 'xml']);

const fileReadCapability: AIChatCapabilityDescriptor = {
  id: 'file.read',
  title: 'Read file',
  description: 'Read file metadata and contents for files visible on the current files page.',
  risk: 'low',
  status: 'available',
  permissions: ['file.view'],
};

const fileListVisibleCapability: AIChatCapabilityDescriptor = {
  id: 'file.list_visible',
  title: 'List visible files',
  description: 'List files visible on the current files page.',
  risk: 'low',
  status: 'available',
  permissions: ['file.view'],
};

const fileDeleteCapability: AIChatCapabilityDescriptor = {
  id: 'file.delete',
  title: 'Delete file',
  description: 'Delete a visible file after explicit approval.',
  risk: 'high',
  requiresConfirmation: true,
  status: 'available',
  permissions: ['file.manage'],
};

const fileCreateCapability: AIChatCapabilityDescriptor = {
  id: 'file.create',
  title: 'Create file',
  description:
    'Create or save a file into File Management only when the user explicitly asks for the current files page or File Management. Generate temporary artifacts with file-generator first, then save them with file-manager/save_file_to_management.',
  risk: 'medium',
  requiresConfirmation: true,
  status: 'available',
  permissions: ['file.upload_create'],
  metadata: {
    target: 'file_management',
    default_without_explicit_target: 'temporary_artifact',
    preferred_skill_ids: 'file-generator,file-manager',
    final_tool: 'file-manager/save_file_to_management',
  },
};

interface VisibleFileContext {
  file: FileItem;
  visibleIndex: number;
  extensionNormalized: string;
  fileTypeNormalized: string;
  extensionRank: number;
  fileTypeRank: number;
  selected: boolean;
}

function filesAIChatCapabilities(
  canManage: boolean,
  canUpload: boolean
): AIChatCapabilityDescriptor[] {
  const capabilities = [fileListVisibleCapability, fileReadCapability];
  if (canUpload) {
    capabilities.push(fileCreateCapability);
  }
  if (canManage) {
    capabilities.push(fileDeleteCapability);
  }
  return capabilities;
}

function compactAIChatContextText(value: string, maxLength = 1200): string {
  const text = value.replace(/\s+/g, ' ').trim();
  if (text.length <= maxLength) return text;
  return `${text.slice(0, maxLength).trim()}...`;
}

function normalizeAIChatFileExtension(file: FileItem): string {
  const explicitExtension = file.extension?.toLowerCase().replace(/^\./, '').trim();
  if (explicitExtension) return explicitExtension;

  const inferredExtension = file.name.split('.').pop()?.toLowerCase().trim();
  if (inferredExtension && inferredExtension !== file.name.toLowerCase()) {
    return inferredExtension;
  }

  return 'unknown';
}

function getAIChatFileType(extension: string): string {
  if (AI_CHAT_EXCEL_EXTENSIONS.has(extension)) return 'excel';
  if (extension === 'pdf') return 'pdf';
  if (AI_CHAT_WORD_EXTENSIONS.has(extension)) return 'word';
  if (AI_CHAT_PRESENTATION_EXTENSIONS.has(extension)) return 'presentation';
  if (extension === 'csv' || extension === 'tsv') return 'spreadsheet';
  if (AI_CHAT_IMAGE_EXTENSIONS.has(extension)) return 'image';
  if (AI_CHAT_AUDIO_EXTENSIONS.has(extension)) return 'audio';
  if (AI_CHAT_VIDEO_EXTENSIONS.has(extension)) return 'video';
  if (AI_CHAT_TEXT_EXTENSIONS.has(extension)) return 'text';

  return extension || 'unknown';
}

function buildAIChatCountSummary(values: string[]): string | null {
  const counts = new Map<string, number>();
  values.forEach(value => {
    counts.set(value, (counts.get(value) ?? 0) + 1);
  });

  const summary = Array.from(counts.entries())
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([value, count]) => `${value}=${count}`)
    .join(',');

  return summary || null;
}

function buildAIChatListMetadata(values: string[]): string | null {
  const filteredValues = values.filter(Boolean);
  return filteredValues.length > 0 ? filteredValues.join(',') : null;
}

function buildVisibleFileIndexMetadata(files: VisibleFileContext[]) {
  const index = files
    .map(({ file, visibleIndex, extensionNormalized, fileTypeNormalized, selected }) =>
      [
        visibleIndex,
        file.id,
        file.name,
        fileTypeNormalized,
        extensionNormalized,
        selected ? 'selected' : '',
      ]
        .filter(Boolean)
        .join(':')
    )
    .join('|');

  return index ? compactAIChatContextText(index, 1800) : null;
}

function buildVisibleFileContextDescription(files: VisibleFileContext[], contextReady: boolean) {
  if (!contextReady) {
    return 'The files list is still loading; do not treat the visible file count as final yet.';
  }

  if (files.length === 0) return 'No files are visible with the current filters.';

  return compactAIChatContextText(
    files
      .map(
        ({
          file,
          visibleIndex,
          extensionNormalized,
          fileTypeNormalized,
          extensionRank,
          fileTypeRank,
          selected,
        }) =>
        [
          `visible_index=${visibleIndex}`,
          `name=${file.name}`,
          `file_type=${fileTypeNormalized}`,
          `extension=${extensionNormalized}`,
          `file_type_rank=${fileTypeRank}`,
          `extension_rank=${extensionRank}`,
          selected ? 'selected=true' : '',
        ]
          .filter(Boolean)
          .join(', ')
      )
      .join(' | '),
    1400
  );
}

function buildFilesPageContextDescription(
  visibleFileContexts: VisibleFileContext[],
  selectedFileIds: string[],
  canUpload: boolean,
  contextReady: boolean,
  pagination: {
    currentPage: number;
    totalPages: number;
    total: number;
    pageSize: number;
    visibleRangeStart: number;
    visibleRangeEnd: number;
  }
) {
  const selectedFileNames = visibleFileContexts
    .filter(({ selected }) => selected)
    .map(({ file }) => file.name)
    .filter(Boolean);
  const selectedSummary =
    selectedFileNames.length > 0
      ? `Selected files: ${selectedFileNames.join(', ')}. `
      : selectedFileIds.length > 0
        ? `${selectedFileIds.length} files are selected. `
      : 'No files are selected. ';
  const ordinalScope =
    'Ordinal references such as fourth file use visible_index; typed ordinal references such as second Excel and last PDF use file_type_rank or extension_rank among visible files of that type. ';
  const createScope = canUpload
    ? 'When the user explicitly asks to create, save, upload, import, or write a file into File Management or the current files page, first generate a temporary artifact when needed, then use file-manager/save_file_to_management. Otherwise generated files remain temporary artifacts. '
    : '';
  const loadingScope = contextReady
    ? ''
    : 'The files query is not settled yet. Wait for page context readiness before answering file count or ordinal file questions. ';
  const pageCountScope = contextReady
    ? `File page count: total_file_count=${pagination.total}, current_page=${pagination.currentPage}/${pagination.totalPages}, page_size=${pagination.pageSize}, visible_range=${pagination.visibleRangeStart}-${pagination.visibleRangeEnd}, indexed_visible_files=${visibleFileContexts.length}. Use total_file_count for the page/table total; use indexed_visible_files only for ordinal references within the current visible page context. `
    : '';

  return compactAIChatContextText(
    `${loadingScope}${pageCountScope}${selectedSummary}${ordinalScope}${createScope}Visible file index: ${buildVisibleFileContextDescription(
      visibleFileContexts,
      contextReady
    )}`,
    1400
  );
}

function buildVisibleFileContexts(files: FileItem[], selectedFileIds: string[]) {
  const selectedFileIdSet = new Set(selectedFileIds);
  const extensionRanks = new Map<string, number>();
  const fileTypeRanks = new Map<string, number>();

  return files.slice(0, FILES_CONTEXT_VISIBLE_LIMIT).map((file, index) => {
    const extensionNormalized = normalizeAIChatFileExtension(file);
    const fileTypeNormalized = getAIChatFileType(extensionNormalized);
    const extensionRank = (extensionRanks.get(extensionNormalized) ?? 0) + 1;
    const fileTypeRank = (fileTypeRanks.get(fileTypeNormalized) ?? 0) + 1;

    extensionRanks.set(extensionNormalized, extensionRank);
    fileTypeRanks.set(fileTypeNormalized, fileTypeRank);

    return {
      file,
      visibleIndex: index + 1,
      extensionNormalized,
      fileTypeNormalized,
      extensionRank,
      fileTypeRank,
      selected: selectedFileIdSet.has(file.id),
    };
  });
}

function visibleFileMetadata({
  file,
  visibleIndex,
  extensionNormalized,
  fileTypeNormalized,
  extensionRank,
  fileTypeRank,
  selected,
}: VisibleFileContext) {
  return {
    page: 'console.files',
    resource_kind: 'file',
    file_id: file.id,
    visible_index: visibleIndex,
    visible_ordinal: visibleIndex,
    visible_rank: visibleIndex,
    display_name: file.name,
    name: file.name,
    extension_normalized: extensionNormalized,
    extension: extensionNormalized,
    extension_original: file.extension,
    file_type: fileTypeNormalized,
    file_type_normalized: fileTypeNormalized,
    file_type_rank: fileTypeRank,
    extension_rank: extensionRank,
    selected,
    workspace_id: file.workspace_id,
    ...(selected
      ? {
          detail_scope: 'selected_file',
          size: file.size,
          mime_type: file.mime_type,
          created_at: file.created_at,
          related_count: file.related_count,
          related_dataset_count: file.related_dataset_count,
        }
      : {
          detail_scope: 'visible_index',
        }),
  };
}

export function buildFilesAIChatContextItems(
  snapshot: FilesAIChatContextSnapshot
): FilesAIChatContextItem[] {
  const {
    files,
    selectedFileIds,
    currentPage,
    totalPages,
    total,
    pageSize,
    sort,
    activeCategory,
    searchValue,
    extensionParam,
    currentWorkspace,
    isOrganizationMode,
    activeFolderName,
    contextReady,
    queryStatus,
    canManage,
    canUpload,
    presentation,
  } = snapshot;
  const capabilities = filesAIChatCapabilities(canManage, canUpload);
  const visibleFileContexts = buildVisibleFileContexts(files, selectedFileIds);
  const selectedVisibleCount = visibleFileContexts.filter(({ selected }) => selected).length;
  const orderedVisibleFileIds = buildAIChatListMetadata(
    visibleFileContexts.map(({ file }) => file.id)
  );
  const selectedFileIdsMetadata = buildAIChatListMetadata(selectedFileIds);
  const fileTypeCounts = buildAIChatCountSummary(
    visibleFileContexts.map(({ fileTypeNormalized }) => fileTypeNormalized)
  );
  const extensionCounts = buildAIChatCountSummary(
    visibleFileContexts.map(({ extensionNormalized }) => extensionNormalized)
  );
  const visibleFileIndex = buildVisibleFileIndexMetadata(visibleFileContexts);
  const selectedVisibleFileIndex = buildVisibleFileIndexMetadata(
    visibleFileContexts.filter(({ selected }) => selected)
  );
  const scopeLabel = isOrganizationMode
    ? 'Personal space'
    : currentWorkspace?.name || 'Current workspace';
  const visibleRangeStart =
    visibleFileContexts.length > 0 ? (currentPage - 1) * pageSize + 1 : 0;
  const visibleRangeEnd =
    visibleFileContexts.length > 0 ? visibleRangeStart + visibleFileContexts.length - 1 : 0;

  return [
    {
      id: 'console.files',
      type: 'page',
      title: '文件管理',
      subtitle: `${scopeLabel} files page`,
      description: buildFilesPageContextDescription(
        visibleFileContexts,
        selectedFileIds,
        canUpload,
        contextReady,
        {
          currentPage,
          totalPages,
          total,
          pageSize,
          visibleRangeStart,
          visibleRangeEnd,
        }
      ),
      href: '/console/files',
      source: 'Files page',
      status: contextReady ? 'available' : 'loading',
      capabilities,
      hints: {
        handledAssetTypes: ['file'],
        toolGovernance: {
          enabled: true,
        },
        presentation,
        refreshHints: [
          { assetType: 'file', queryKey: [FILES_QUERY_KEY] },
          { assetType: 'file', queryKey: [FILE_FOLDERS_KEY] },
          { assetType: 'file', queryKey: [STORAGE_USAGE_KEY] },
          { assetType: 'file', queryKey: FILE_KEYS.all },
        ],
      },
      metadata: {
        page: 'console.files',
        route: '/console/files',
        resource_kind: 'page',
        context_ready: contextReady,
        files_query_status: queryStatus,
        files_query_settled: contextReady,
        ordered_visible_file_ids: orderedVisibleFileIds,
        selected_file_ids: selectedFileIdsMetadata,
        visible_file_index: visibleFileIndex,
        selected_visible_file_index: selectedVisibleFileIndex,
        visible_file_count: visibleFileContexts.length,
        selected_file_count: selectedFileIds.length,
        selected_visible_file_count: selectedVisibleCount,
        file_type_counts: fileTypeCounts,
        extension_counts: extensionCounts,
        current_page: currentPage,
        page_size: pageSize,
        visible_range_start: visibleRangeStart,
        visible_range_end: visibleRangeEnd,
        more_pages_available: currentPage < totalPages,
        context_visible_limit: FILES_CONTEXT_VISIBLE_LIMIT,
        omitted_context_file_count: Math.max(files.length - visibleFileContexts.length, 0),
        ordinal_scope: 'current_visible_page',
        visible_order_basis: 'current_visible_page_order',
        sort,
        sort_key: FILES_PAGE_SORT_KEY,
        sort_direction: FILES_PAGE_SORT_DIRECTION,
        category: activeCategory,
        total_file_count: total,
        total_pages: totalPages,
        folder_name: activeFolderName,
        search: searchValue.trim(),
        extension_filter: extensionParam,
        workspace_id: isOrganizationMode ? undefined : currentWorkspace?.id,
        workspace_name: isOrganizationMode ? undefined : currentWorkspace?.name,
        organization_mode: isOrganizationMode,
      },
    },
    ...visibleFileContexts.map(context => ({
      id: context.file.id,
      type: 'file' as const,
      title: context.file.name,
      subtitle: `${context.fileTypeNormalized} - ${context.extensionNormalized}`,
      description: `Visible file ${context.visibleIndex} on console.files page. Use read_file to inspect content.`,
      href: '/console/files',
      source: 'Files page',
      status: 'available' as const,
      capabilities,
      metadata: visibleFileMetadata(context),
    })),
  ];
}
