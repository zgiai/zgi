'use client';

import { memo } from 'react';
import {
  MessageSquare,
  Eye,
  Image as ImageIcon,
  Layers,
  Search,
  Mic,
  Volume2,
  Zap,
  Video,
  ShieldCheck,
  Brain,
  Wrench,
  Bot,
  HelpCircle,
  type LucideIcon,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import type { ModelUseCase } from '@/services/types/model';
import { USE_CASE_BASE_COLORS } from '@/config/model-colors';

const USE_CASE_ICONS: Record<string, LucideIcon> = {
  'text-chat': MessageSquare,
  vision: Eye,
  'image-gen': ImageIcon,
  embedding: Layers,
  rerank: Search,
  'speech-to-text': Mic,
  'text-to-speech': Volume2,
  'realtime-audio': Zap,
  'video-gen': Video,
  moderation: ShieldCheck,
  reasoning: Brain,
  'function-calling': Wrench,
  agent: Bot,
  unknown: HelpCircle,
};

// Map base color names to Tailwind text color classes
const COLOR_MAP: Record<string, string> = {
  blue: 'text-blue-500',
  orange: 'text-orange-500',
  pink: 'text-pink-500',
  violet: 'text-violet-500',
  amber: 'text-amber-500',
  teal: 'text-teal-500',
  lime: 'text-lime-500',
  sky: 'text-sky-500',
  rose: 'text-rose-500',
  red: 'text-red-500',
  emerald: 'text-emerald-500',
  indigo: 'text-indigo-500',
};

export interface ModelUseCaseIconProps {
  useCase: string;
  className?: string;
  colored?: boolean;
}

export const ModelUseCaseIcon = memo(function ModelUseCaseIcon({
  useCase,
  className,
  colored = true,
}: ModelUseCaseIconProps) {
  const Icon = USE_CASE_ICONS[useCase] || USE_CASE_ICONS.unknown;
  const baseColor = USE_CASE_BASE_COLORS[useCase as ModelUseCase];
  const colorClass = colored && baseColor ? COLOR_MAP[baseColor] : '';

  return <Icon className={cn('h-3.5 w-3.5', colorClass, className)} />;
});
