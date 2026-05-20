function escapeHtml(input: string): string {
  return input
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

export function transformThinkTags(input: string, summaryLabel: string): string {
  const safeSummary = escapeHtml(summaryLabel);
  const startWrapper = `<details class="md-think-block" open><summary class="md-think-header"><span class="md-think-summary-left"><span class="md-think-icon-dot"></span><span>${safeSummary}</span></span></summary><div class="md-think-content">`;
  const endWrapper = '</div></details>';

  const normalizeSegment = (segment: string): string => {
    const tokenRegex = /<think(?:\s[^>]*)?>|<\/think\s*>/gi;
    let depth = 0;
    let out = '';
    let lastIndex = 0;
    let match: RegExpExecArray | null;

    while ((match = tokenRegex.exec(segment)) !== null) {
      out += segment.slice(lastIndex, match.index);
      const token = match[0];
      if (/^<think(?:\s[^>]*)?>$/i.test(token)) {
        depth += 1;
        if (depth === 1) out += startWrapper;
      } else if (depth > 0) {
        depth -= 1;
        if (depth === 0) out += endWrapper;
      }
      lastIndex = tokenRegex.lastIndex;
    }

    out += segment.slice(lastIndex);
    if (depth > 0) out += endWrapper;
    return out;
  };

  const fenceRegex = /```[\s\S]*?```/g;
  let output = '';
  let cursor = 0;
  let match: RegExpExecArray | null;

  while ((match = fenceRegex.exec(input)) !== null) {
    const segment = input.slice(cursor, match.index);
    output += normalizeSegment(segment);
    output += match[0];
    cursor = match.index + match[0].length;
  }

  output += normalizeSegment(input.slice(cursor));
  return output;
}
