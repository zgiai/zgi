/**
 * Resolve a recognition request only when it still belongs to the latest analysis.
 *
 * @template T
 * @param {{
 *   recognitionRequestSeqRef: { current: number };
 *   currentAnalysisKeyRef: { current: string };
 *   analysisKey: string;
 *   request: () => Promise<T>;
 * }} options
 * @returns {Promise<T | null>}
 */
export async function runLatestRecognition({
  recognitionRequestSeqRef,
  currentAnalysisKeyRef,
  analysisKey,
  request,
}) {
  const requestSeq = recognitionRequestSeqRef.current + 1;
  recognitionRequestSeqRef.current = requestSeq;

  const result = await request();

  if (
    requestSeq !== recognitionRequestSeqRef.current ||
    analysisKey !== currentAnalysisKeyRef.current
  ) {
    return null;
  }

  return result;
}
