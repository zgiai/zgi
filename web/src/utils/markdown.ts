interface RangeMatch {
  nextIndex: number;
  text: string;
}

const HTML_RAW_BLOCK_TAGS = ['pre', 'code', 'script', 'style', 'svg'] as const;

function findLineStart(input: string, index: number): number {
  let cursor = index;
  while (cursor > 0 && input[cursor - 1] !== '\n') {
    cursor -= 1;
  }
  return cursor;
}

function findLineEnd(input: string, index: number): number {
  let cursor = index;
  while (cursor < input.length && input[cursor] !== '\n') {
    cursor += 1;
  }
  return cursor;
}

function countSequential(input: string, index: number, marker: string): number {
  let cursor = index;
  while (input[cursor] === marker) {
    cursor += 1;
  }
  return cursor - index;
}

function isEscaped(input: string, index: number): boolean {
  let slashCount = 0;
  let cursor = index - 1;

  while (cursor >= 0 && input[cursor] === '\\') {
    slashCount += 1;
    cursor -= 1;
  }

  return slashCount % 2 === 1;
}

function isWhitespaceOnly(input: string): boolean {
  return /^[\t \r]*$/.test(input);
}

function findUnescapedToken(input: string, token: string, fromIndex: number): number {
  let cursor = fromIndex;

  while (cursor < input.length) {
    const nextIndex = input.indexOf(token, cursor);
    if (nextIndex === -1) return -1;
    if (!isEscaped(input, nextIndex)) return nextIndex;
    cursor = nextIndex + 1;
  }

  return -1;
}

function trimBlankLines(lines: string[]): string[] {
  let start = 0;
  let end = lines.length;

  while (start < end && lines[start].trim().length === 0) {
    start += 1;
  }

  while (end > start && lines[end - 1].trim().length === 0) {
    end -= 1;
  }

  return lines.slice(start, end);
}

function normalizeBlockMathBody(body: string): string {
  const normalized = body.replace(/\r\n?/g, '\n');
  const trimmedLines = trimBlankLines(normalized.split('\n'));

  if (trimmedLines.length === 0) return '';
  if (trimmedLines.length === 1) return trimmedLines[0].trim();

  return trimmedLines.join('\n');
}

function buildNormalizedBlockMath(body: string, indent: string): string {
  const normalizedBody = normalizeBlockMathBody(body);
  if (!normalizedBody) {
    return `$$\n${indent}$$`;
  }

  const indentedBody = normalizedBody.split('\n').join(`\n${indent}`);
  return `$$\n${indent}${indentedBody}\n${indent}$$`;
}

function readHtmlComment(input: string, index: number): RangeMatch | null {
  if (!input.startsWith('<!--', index)) return null;

  const closeIndex = input.indexOf('-->', index + 4);
  const nextIndex = closeIndex === -1 ? input.length : closeIndex + 3;

  return {
    text: input.slice(index, nextIndex),
    nextIndex,
  };
}

function readRawHtmlBlock(input: string, index: number): RangeMatch | null {
  if (input[index] !== '<') return null;

  const remainder = input.slice(index).toLowerCase();
  const tag = HTML_RAW_BLOCK_TAGS.find(candidate => {
    const prefix = `<${candidate}`;
    const nextChar = remainder[prefix.length];
    return remainder.startsWith(prefix) && (nextChar === undefined || /[\s>/]/.test(nextChar));
  });

  if (!tag) return null;

  const openTagEnd = input.indexOf('>', index + 1);
  if (openTagEnd === -1) {
    return {
      text: input.slice(index),
      nextIndex: input.length,
    };
  }

  const lowerInput = input.toLowerCase();
  const closeToken = `</${tag}`;
  const closeIndex = lowerInput.indexOf(closeToken, openTagEnd + 1);

  if (closeIndex === -1) {
    return {
      text: input.slice(index),
      nextIndex: input.length,
    };
  }

  const closeTagEnd = input.indexOf('>', closeIndex + closeToken.length);
  if (closeTagEnd === -1) {
    return {
      text: input.slice(index),
      nextIndex: input.length,
    };
  }

  return {
    text: input.slice(index, closeTagEnd + 1),
    nextIndex: closeTagEnd + 1,
  };
}

function readInlineCodeSpan(input: string, index: number): RangeMatch | null {
  if (input[index] !== '`') return null;

  const tickCount = countSequential(input, index, '`');
  let cursor = index + tickCount;

  while (cursor < input.length) {
    const nextTick = input.indexOf('`', cursor);
    if (nextTick === -1) return null;

    const nextTickCount = countSequential(input, nextTick, '`');
    if (nextTickCount === tickCount) {
      return {
        text: input.slice(index, nextTick + tickCount),
        nextIndex: nextTick + tickCount,
      };
    }

    cursor = nextTick + nextTickCount;
  }

  return null;
}

function readFencedCodeBlock(input: string, index: number): RangeMatch | null {
  const marker = input[index];
  if (marker !== '`' && marker !== '~') return null;

  const lineStart = findLineStart(input, index);
  const linePrefix = input.slice(lineStart, index);
  if (!/^[\t ]{0,3}$/.test(linePrefix)) return null;

  const fenceLength = countSequential(input, index, marker);
  if (fenceLength < 3) return null;

  let cursor = findLineEnd(input, index);
  if (cursor < input.length && input[cursor] === '\n') {
    cursor += 1;
  }

  while (cursor < input.length) {
    const closeLineEnd = findLineEnd(input, cursor);
    let closeCursor = cursor;

    while (closeCursor < closeLineEnd && (input[closeCursor] === ' ' || input[closeCursor] === '\t')) {
      closeCursor += 1;
    }

    if (closeCursor - cursor <= 3 && input[closeCursor] === marker) {
      const closingLength = countSequential(input, closeCursor, marker);
      const trailing = input.slice(closeCursor + closingLength, closeLineEnd);

      if (closingLength >= fenceLength && isWhitespaceOnly(trailing)) {
        return {
          text: input.slice(index, closeLineEnd),
          nextIndex: closeLineEnd,
        };
      }
    }

    cursor = closeLineEnd;
    if (cursor < input.length && input[cursor] === '\n') {
      cursor += 1;
    }
  }

  return {
    text: input.slice(index),
    nextIndex: input.length,
  };
}

function getBlockIndent(input: string, index: number): string | null {
  const lineStart = findLineStart(input, index);
  const indent = input.slice(lineStart, index);
  return isWhitespaceOnly(indent) ? indent : null;
}

function readInlineParenMath(input: string, index: number): RangeMatch | null {
  if (!input.startsWith('\\(', index) || isEscaped(input, index)) return null;

  const closeIndex = findUnescapedToken(input, '\\)', index + 2);
  if (closeIndex === -1) return null;

  return {
    text: `$${input.slice(index + 2, closeIndex)}$`,
    nextIndex: closeIndex + 2,
  };
}

function readBlockBracketMath(input: string, index: number): RangeMatch | null {
  if (!input.startsWith('\\[', index) || isEscaped(input, index)) return null;

  const closeIndex = findUnescapedToken(input, '\\]', index + 2);
  if (closeIndex === -1) return null;

  const indent = getBlockIndent(input, index);
  if (indent === null) return null;

  const closeLineEnd = findLineEnd(input, closeIndex + 2);
  const closeSuffix = input.slice(closeIndex + 2, closeLineEnd);
  if (!isWhitespaceOnly(closeSuffix)) return null;

  return {
    text: buildNormalizedBlockMath(input.slice(index + 2, closeIndex), indent),
    nextIndex: closeLineEnd,
  };
}

function readBlockDollarMath(input: string, index: number): RangeMatch | null {
  if (!input.startsWith('$$', index) || isEscaped(input, index)) return null;

  const closeIndex = findUnescapedToken(input, '$$', index + 2);
  if (closeIndex === -1) return null;

  const indent = getBlockIndent(input, index);
  if (indent === null) return null;

  const closeLineEnd = findLineEnd(input, closeIndex + 2);
  const closeSuffix = input.slice(closeIndex + 2, closeLineEnd);
  if (!isWhitespaceOnly(closeSuffix)) return null;

  return {
    text: buildNormalizedBlockMath(input.slice(index + 2, closeIndex), indent),
    nextIndex: closeLineEnd,
  };
}

/**
 * @util Normalize supported LaTeX-style math delimiters into the markdown math
 * formats accepted by the current renderer without touching protected regions.
 */
export function normalizeMarkdownMathDelimiters(markdown: string): string {
  if (!markdown) return '';

  let output = '';
  let cursor = 0;

  while (cursor < markdown.length) {
    const protectedMatch =
      readHtmlComment(markdown, cursor) ||
      readRawHtmlBlock(markdown, cursor) ||
      readFencedCodeBlock(markdown, cursor) ||
      readInlineCodeSpan(markdown, cursor);

    if (protectedMatch) {
      output += protectedMatch.text;
      cursor = protectedMatch.nextIndex;
      continue;
    }

    const mathMatch =
      readBlockBracketMath(markdown, cursor) ||
      readBlockDollarMath(markdown, cursor) ||
      readInlineParenMath(markdown, cursor);

    if (mathMatch) {
      output += mathMatch.text;
      cursor = mathMatch.nextIndex;
      continue;
    }

    output += markdown[cursor];
    cursor += 1;
  }

  return output;
}
