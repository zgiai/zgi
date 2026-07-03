import type { SegmentDetail } from '@/services/types/dataset';

/**
 * Props for SegmentCard component
 * Simplified: questions and inline child-segment CRUD removed
 */
export interface SegmentCardProps {
  segment: SegmentDetail;
  /** Calculated display index based on pagination (1-indexed) */
  displayIndex: number;
  isSelected: boolean;
  isExpanded: boolean;
  onSelect: (segmentId: string) => void;
  onToggleExpand: (segmentId: string) => void;
  onToggleEnabled: (segmentIds: string[], enabled: boolean) => void;
  onEdit: (segment: SegmentDetail) => void;
  onDelete: (segmentIds: string[]) => void;
  onViewChildChunks: (segment: SegmentDetail) => void;
  onViewAllChildSegments: (segment: SegmentDetail) => void;
  onEditChildSegment: (segmentId: string, childChunkId: string, content: string) => void;
  onDeleteChildSegment: (segmentId: string, childChunkId: string) => void;
  readOnly?: boolean;
}

/**
 * Props for SegmentHeader component
 */
export interface SegmentHeaderProps {
  segment: SegmentDetail;
  /** Calculated display index based on pagination (1-indexed) */
  displayIndex: number;
  isSelected: boolean;
  onSelect: (segmentId: string) => void;
  onToggleEnabled: (segmentIds: string[], enabled: boolean) => void;
  onEdit: (segment: SegmentDetail) => void;
  onDelete: (segmentIds: string[]) => void;
  onViewChildChunks?: (segment: SegmentDetail) => void;
  readOnly?: boolean;
}

/**
 * Props for SecondaryChunks component
 * Inline preview with edit/delete actions for each child segment
 */
export interface SecondaryChunksProps {
  segment: SegmentDetail;
  onViewAllChildSegments: (segment: SegmentDetail) => void;
  onEditChildSegment: (segmentId: string, childChunkId: string, content: string) => void;
  onDeleteChildSegment: (segmentId: string, childChunkId: string) => void;
  readOnly?: boolean;
}

export interface QuestionItem {
  id: string;
  question: string;
}

export interface QuestionsSectionProps {
  segment: SegmentDetail;
  isExpanded: boolean;
  questionsLoading: boolean;
  questionsData: QuestionItem[] | undefined;
  generatingQuestionsSegmentId: string | null;
  onToggleExpand: (segmentId: string) => void;
  onPrefetchQuestions: (segmentId: string) => void;
  onAddQuestion: (segment: SegmentDetail) => void;
  onGenerateQuestions: (segmentId: string) => void;
  onBatchImportQuestions: (segmentId: string) => void;
  onEditQuestion: (segmentId: string, questionId: string, question: string) => void;
  onDeleteQuestion: (segmentId: string, questionId: string) => void;
}
