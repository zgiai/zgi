/**
 * Invalidate any in-flight recognition before starting a new analysis.
 *
 * @param {{ current: number }} recognitionRequestSeqRef
 * @param {{ current: string }} currentAnalysisKeyRef
 */
export function invalidateRecognitionAnalysis(
  recognitionRequestSeqRef,
  currentAnalysisKeyRef
) {
  recognitionRequestSeqRef.current += 1;
  currentAnalysisKeyRef.current = '';
}

/**
 * Mark an analyzed workbook sheet as the active recognition target.
 *
 * @param {{ current: string }} currentAnalysisKeyRef
 * @param {string} analysisKey
 */
export function activateRecognitionAnalysis(currentAnalysisKeyRef, analysisKey) {
  currentAnalysisKeyRef.current = analysisKey;
}

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
