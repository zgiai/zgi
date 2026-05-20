export interface TextStreamThrottler {
  append: (text: string) => void;
  flush: () => void;
  cancel: () => void;
}

export function createTextStreamThrottler(
  interval: number,
  apply: (text: string) => void
): TextStreamThrottler {
  let buffer = '';
  let timer: ReturnType<typeof setTimeout> | null = null;

  const runApply = (): void => {
    if (buffer.length === 0) return;
    const payload = buffer;
    buffer = '';
    apply(payload);
  };

  const append = (text: string): void => {
    if (text.length === 0) return;
    buffer += text;
    if (timer === null) {
      timer = setTimeout(() => {
        timer = null;
        runApply();
      }, interval);
    }
  };

  const flush = (): void => {
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
    runApply();
  };

  const cancel = (): void => {
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
    buffer = '';
  };

  return { append, flush, cancel };
}
