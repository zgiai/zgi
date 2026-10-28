import {
  BookOpen,
  Briefcase,
  ClipboardList,
  FolderKanban,
  Heart,
  HeartPulse,
  Dumbbell,
  Plane,
  StickyNote,
  UserRound,
  Users,
  type LucideIcon,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { knownTopicOf, type KnownMemoryTopic } from './memory-topics';

const topicIcons: Record<KnownMemoryTopic, { icon: LucideIcon; tone: string }> = {
  profile: { icon: UserRound, tone: 'bg-violet-500/10 text-violet-600 dark:text-violet-400' },
  preferences: { icon: Heart, tone: 'bg-pink-500/10 text-pink-600 dark:text-pink-400' },
  standing_instructions: {
    icon: ClipboardList,
    tone: 'bg-amber-500/10 text-amber-600 dark:text-amber-400',
  },
  project_context: {
    icon: FolderKanban,
    tone: 'bg-sky-500/10 text-sky-600 dark:text-sky-400',
  },
  projects: { icon: FolderKanban, tone: 'bg-sky-500/10 text-sky-600 dark:text-sky-400' },
  work: { icon: Briefcase, tone: 'bg-teal-500/10 text-teal-600 dark:text-teal-400' },
  health: { icon: HeartPulse, tone: 'bg-rose-500/10 text-rose-600 dark:text-rose-400' },
  fitness: { icon: Dumbbell, tone: 'bg-orange-500/10 text-orange-600 dark:text-orange-400' },
  family: { icon: Users, tone: 'bg-indigo-500/10 text-indigo-600 dark:text-indigo-400' },
  learning: { icon: BookOpen, tone: 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400' },
  travel: { icon: Plane, tone: 'bg-cyan-500/10 text-cyan-600 dark:text-cyan-400' },
  notes: { icon: StickyNote, tone: 'bg-muted text-muted-foreground' },
};

/** A small tinted icon that tells topics apart at a glance. */
export function MemoryTopicIcon({ topicKey, className }: { topicKey: string; className?: string }) {
  const { icon: Icon, tone } = topicIcons[knownTopicOf(topicKey) ?? 'notes'];
  return (
    <span
      aria-hidden="true"
      className={cn('flex size-7 shrink-0 items-center justify-center rounded-lg', tone, className)}
    >
      <Icon className="size-3.5" />
    </span>
  );
}
