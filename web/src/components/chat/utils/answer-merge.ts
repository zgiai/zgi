export type AnswerMergeMode = 'append' | 'replace' | 'skip';

export function resolveAnswerMergeMode(
  currentAnswer: string,
  incomingAnswer: string
): AnswerMergeMode {
  if (!incomingAnswer) return 'skip';
  if (!currentAnswer) return 'append';
  if (incomingAnswer === currentAnswer) return 'skip';
  if (incomingAnswer.startsWith(currentAnswer)) return 'replace';
  if (currentAnswer.endsWith(incomingAnswer)) return 'skip';
  return 'append';
}

export function shouldPreserveLocalAnswerForSnapshot(
  localAnswer: string,
  snapshotAnswer: string
): boolean {
  return localAnswer.length > snapshotAnswer.length && localAnswer.startsWith(snapshotAnswer);
}
