'use client';

import { memo } from 'react';
import {
  Wrench,
  Paperclip,
  Brain,
  FileJson,
  Thermometer,
  Eye,
  MessageSquare,
  FileSearch2,
  Code2,
  Computer,
  Zap,
  Globe,
  Mic,
  Volume2,
  Image,
  ShieldCheck,
  Workflow,
  type LucideIcon,
  Braces,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { FEATURE_ICON_COLORS, DEFAULT_FEATURE_COLOR } from '@/config/model-colors';

// Dynamic feature key type - can be any string from the API
export type ModelFeatureKey = string;

// Icon mapping for common feature keys
const FEATURE_ICONS: Record<string, LucideIcon> = {
  // Features
  streaming: Zap,
  function_calling: Wrench,
  structured_output: FileJson,
  json_mode: Braces,
  distillation: Workflow,
  reasoning: Brain,
  system_prompt: MessageSquare,
  logprobs: Code2,
  web_search: Globe,
  file_search: FileSearch2,
  code_interpreter: Code2,
  computer_use: Computer,
  mcp: Workflow,
  reasoning_effort: Brain,
  attachment: Paperclip,
  // Endpoints
  chat_completions: MessageSquare,
  responses: MessageSquare,
  realtime: Zap,
  assistants: Brain,
  batch: Workflow,
  embeddings: Workflow,
  fine_tuning: Workflow,
  image_generation: Image,
  vision: Eye,
  speech_generation: Volume2,
  transcription: Mic,
  translation: Globe,
  moderation: ShieldCheck,
  // Tools
  parallel_tool_calls: Wrench,
  // Legacy mappings for backward compatibility
  tool: Wrench,
  structured: FileJson,
  temperature: Thermometer,
};

// Default icon for unknown features
const DefaultIcon = Zap;

export interface ModelFeatureIconProps {
  feature: ModelFeatureKey;
  className?: string;
  /** If true, applies the feature-specific color. Default: true */
  colored?: boolean;
}

export const ModelFeatureIcon = memo(function ModelFeatureIcon({
  feature,
  className,
  colored = true,
}: ModelFeatureIconProps) {
  const Icon = FEATURE_ICONS[feature] || DefaultIcon;
  const colorClass = colored ? FEATURE_ICON_COLORS[feature] || DEFAULT_FEATURE_COLOR : '';
  return <Icon className={cn('h-3.5 w-3.5', colorClass, className)} />;
});
