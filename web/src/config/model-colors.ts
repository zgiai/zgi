import type { ModelUseCase } from '@/services/types/model';

/**
 * Centralized color configuration for model use cases
 * Used across ModelTypeChips, ModelsGroupTable, and other components
 *
 * Color assignments (maximized distinction with warm colors):
 * - text-chat:      indigo    (primary conversation)
 * - vision:         cyan      (image understanding)
 * - image-gen:      fuchsia   (image creation)
 * - embedding:      violet    (vector/semantic)
 * - rerank:         slate     (search optimization)
 * - speech-to-text: emerald   (audio input)
 * - text-to-speech: sky       (audio output)
 * - realtime-audio: amber     (live audio)
 * - video-gen:      rose      (video creation)
 * - moderation:     red       (safety/filtering)
 * - reasoning:      purple    (deep wisdom)
 * - function-calling: orange  (utility/action)
 */

// Base color definitions for each use case
export const USE_CASE_BASE_COLORS: Record<ModelUseCase, string> = {
  'text-chat': 'indigo',
  vision: 'cyan',
  'image-gen': 'fuchsia',
  embedding: 'violet',
  rerank: 'slate',
  'speech-to-text': 'emerald',
  'text-to-speech': 'sky',
  'realtime-audio': 'amber',
  'video-gen': 'rose',
  moderation: 'red',
  reasoning: 'purple',
  'function-calling': 'orange',
};

// Selected state colors (filled background) for chips/buttons
export const USE_CASE_SELECTED_COLORS: Record<ModelUseCase, string> = {
  'text-chat': 'bg-indigo-600 text-white border-indigo-600 hover:bg-indigo-700',
  vision: 'bg-cyan-600 text-white border-cyan-600 hover:bg-cyan-700',
  'image-gen': 'bg-fuchsia-600 text-white border-fuchsia-600 hover:bg-fuchsia-700',
  embedding: 'bg-violet-600 text-white border-violet-600 hover:bg-violet-700',
  rerank: 'bg-slate-600 text-white border-slate-600 hover:bg-slate-700',
  'speech-to-text': 'bg-emerald-600 text-white border-emerald-600 hover:bg-emerald-700',
  'text-to-speech': 'bg-sky-600 text-white border-sky-600 hover:bg-sky-700',
  'realtime-audio': 'bg-amber-600 text-white border-amber-600 hover:bg-amber-700',
  'video-gen': 'bg-rose-600 text-white border-rose-600 hover:bg-rose-700',
  moderation: 'bg-red-600 text-white border-red-600 hover:bg-red-700',
  reasoning: 'bg-purple-600 text-white border-purple-600 hover:bg-purple-700',
  'function-calling': 'bg-orange-600 text-white border-orange-600 hover:bg-orange-700',
};

// Unselected state colors (subtle background) for chips/buttons
export const USE_CASE_UNSELECTED_COLORS: Record<ModelUseCase, string> = {
  'text-chat':
    'bg-indigo-50 text-indigo-700 border-indigo-200 hover:bg-indigo-100 dark:bg-indigo-950/50 dark:text-indigo-400 dark:border-indigo-800 dark:hover:bg-indigo-900/50',
  vision:
    'bg-cyan-50 text-cyan-700 border-cyan-200 hover:bg-cyan-100 dark:bg-cyan-950/50 dark:text-cyan-400 dark:border-cyan-800 dark:hover:bg-cyan-900/50',
  'image-gen':
    'bg-fuchsia-50 text-fuchsia-700 border-fuchsia-200 hover:bg-fuchsia-100 dark:bg-fuchsia-950/50 dark:text-fuchsia-400 dark:border-fuchsia-800 dark:hover:bg-fuchsia-900/50',
  embedding:
    'bg-violet-50 text-violet-700 border-violet-200 hover:bg-violet-100 dark:bg-violet-950/50 dark:text-violet-400 dark:border-violet-800 dark:hover:bg-violet-900/50',
  rerank:
    'bg-slate-50 text-slate-700 border-slate-200 hover:bg-slate-100 dark:bg-slate-950/50 dark:text-slate-400 dark:border-slate-800 dark:hover:bg-slate-900/50',
  'speech-to-text':
    'bg-emerald-50 text-emerald-700 border-emerald-200 hover:bg-emerald-100 dark:bg-emerald-950/50 dark:text-emerald-400 dark:border-emerald-800 dark:hover:bg-emerald-900/50',
  'text-to-speech':
    'bg-sky-50 text-sky-700 border-sky-200 hover:bg-sky-100 dark:bg-sky-950/50 dark:text-sky-400 dark:border-sky-800 dark:hover:bg-sky-900/50',
  'realtime-audio':
    'bg-amber-50 text-amber-700 border-amber-200 hover:bg-amber-100 dark:bg-amber-950/50 dark:text-amber-400 dark:border-amber-800 dark:hover:bg-amber-900/50',
  'video-gen':
    'bg-rose-50 text-rose-700 border-rose-200 hover:bg-rose-100 dark:bg-rose-950/50 dark:text-rose-400 dark:border-rose-800 dark:hover:bg-rose-900/50',
  moderation:
    'bg-red-50 text-red-700 border-red-200 hover:bg-red-100 dark:bg-red-950/50 dark:text-red-400 dark:border-red-800 dark:hover:bg-red-900/50',
  reasoning:
    'bg-purple-50 text-purple-700 border-purple-200 hover:bg-purple-100 dark:bg-purple-950/50 dark:text-purple-400 dark:border-purple-800 dark:hover:bg-purple-900/50',
  'function-calling':
    'bg-orange-50 text-orange-700 border-orange-200 hover:bg-orange-100 dark:bg-orange-950/50 dark:text-orange-400 dark:border-orange-800 dark:hover:bg-orange-900/50',
};

// Badge colors for table rows (outline style)
export const USE_CASE_BADGE_COLORS: Record<ModelUseCase, string> = {
  'text-chat':
    'bg-indigo-100 text-indigo-700 border-indigo-200 dark:bg-indigo-900/30 dark:text-indigo-400 dark:border-indigo-800',
  vision:
    'bg-cyan-100 text-cyan-700 border-cyan-200 dark:bg-cyan-900/30 dark:text-cyan-400 dark:border-cyan-800',
  'image-gen':
    'bg-fuchsia-100 text-fuchsia-700 border-fuchsia-200 dark:bg-fuchsia-900/30 dark:text-fuchsia-400 dark:border-fuchsia-800',
  embedding:
    'bg-violet-100 text-violet-700 border-violet-200 dark:bg-violet-900/30 dark:text-violet-400 dark:border-violet-800',
  rerank:
    'bg-slate-100 text-slate-700 border-slate-200 dark:bg-slate-900/30 dark:text-slate-400 dark:border-slate-800',
  'speech-to-text':
    'bg-emerald-100 text-emerald-700 border-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-400 dark:border-emerald-800',
  'text-to-speech':
    'bg-sky-100 text-sky-700 border-sky-200 dark:bg-sky-900/30 dark:text-sky-400 dark:border-sky-800',
  'realtime-audio':
    'bg-amber-100 text-amber-700 border-amber-200 dark:bg-amber-900/30 dark:text-amber-400 dark:border-amber-800',
  'video-gen':
    'bg-rose-100 text-rose-700 border-rose-200 dark:bg-rose-900/30 dark:text-rose-400 dark:border-rose-800',
  moderation:
    'bg-red-100 text-red-700 border-red-200 dark:bg-red-900/30 dark:text-red-400 dark:border-red-800',
  reasoning:
    'bg-purple-100 text-purple-700 border-purple-200 dark:bg-purple-900/30 dark:text-purple-400 dark:border-purple-800',
  'function-calling':
    'bg-orange-100 text-orange-700 border-orange-200 dark:bg-orange-900/30 dark:text-orange-400 dark:border-orange-800',
};

// Model feature icon colors
export const FEATURE_ICON_COLORS: Record<string, string> = {
  // Core capabilities - Blue/Indigo
  streaming: 'text-yellow-500',
  function_calling: 'text-blue-500',
  structured_output: 'text-violet-500',
  json_mode: 'text-green-700',

  // AI/Intelligence - Purple/Pink
  reasoning: 'text-purple-500',
  reasoning_effort: 'text-purple-400',
  distillation: 'text-pink-500',

  // Communication - Green
  system_prompt: 'text-green-500',
  chat_completions: 'text-green-500',
  responses: 'text-green-400',

  // Search/Discovery - Cyan/Teal
  web_search: 'text-cyan-500',
  file_search: 'text-teal-500',

  // Code/Technical - Orange/Amber
  code_interpreter: 'text-orange-500',
  logprobs: 'text-amber-500',

  // Multimodal - Rose/Red
  vision: 'text-rose-500',
  image_generation: 'text-rose-400',
  attachment: 'text-red-400',

  // Audio - Sky
  speech_generation: 'text-sky-500',
  transcription: 'text-sky-400',
  translation: 'text-sky-300',

  // System/Control - Slate/Gray
  computer_use: 'text-slate-500',
  mcp: 'text-gray-500',
  moderation: 'text-slate-600',

  // Realtime/Speed - Yellow
  realtime: 'text-yellow-500',

  // Tools - Emerald
  parallel_tool_calls: 'text-emerald-500',
  tool: 'text-emerald-500',

  // Other
  assistants: 'text-purple-400',
  batch: 'text-gray-400',
  embeddings: 'text-indigo-400',
  fine_tuning: 'text-amber-400',
  temperature: 'text-orange-400',
  structured: 'text-violet-500',
};

// Default color for unknown features
export const DEFAULT_FEATURE_COLOR = 'text-muted-foreground';

// Ordered list of use cases for display
export const USE_CASE_ORDER: ModelUseCase[] = [
  'text-chat',
  'vision',
  'image-gen',
  'embedding',
  'rerank',
  'speech-to-text',
  'text-to-speech',
  'realtime-audio',
  'video-gen',
  'moderation',
  'reasoning',
  'function-calling',
];
