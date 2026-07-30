export async function runUploadPool<T, R>(
  items: readonly T[],
  concurrency: number,
  upload: (item: T, index: number) => Promise<R>,
  shouldContinue: () => boolean = () => true
): Promise<Array<R | undefined>> {
  if (items.length === 0) {
    return [];
  }

  const normalizedConcurrency = Number.isFinite(concurrency)
    ? Math.max(1, Math.floor(concurrency))
    : 1;
  const workerCount = Math.min(normalizedConcurrency, items.length);
  const results: Array<R | undefined> = new Array(items.length);
  let nextIndex = 0;

  const worker = async () => {
    while (shouldContinue()) {
      const currentIndex = nextIndex;
      nextIndex += 1;
      if (currentIndex >= items.length) {
        return;
      }

      results[currentIndex] = await upload(items[currentIndex], currentIndex);
    }
  };

  await Promise.all(Array.from({ length: workerCount }, () => worker()));
  return results;
}
