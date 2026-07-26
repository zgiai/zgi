interface MarkdownAstNode {
  type: string;
  value?: unknown;
  children?: MarkdownAstNode[];
  [key: string]: unknown;
}

function splitTextSoftBreaks(node: MarkdownAstNode): MarkdownAstNode[] {
  if (node.type !== 'text' || typeof node.value !== 'string' || !/[\r\n]/.test(node.value)) {
    return [node];
  }

  const parts = node.value.split(/\r\n?|\n/);
  const result: MarkdownAstNode[] = [];
  for (const [index, part] of parts.entries()) {
    if (index > 0) result.push({ type: 'break' });
    if (part) result.push({ ...node, value: part });
  }
  return result;
}

function transformSoftBreaks(node: MarkdownAstNode): void {
  if (!Array.isArray(node.children)) return;

  const children: MarkdownAstNode[] = [];
  for (const child of node.children) {
    if (child.type === 'text') {
      children.push(...splitTextSoftBreaks(child));
      continue;
    }
    transformSoftBreaks(child);
    children.push(child);
  }
  node.children = children;
}

/**
 * Treat CommonMark soft line breaks as visible line breaks across every
 * Markdown surface. Fenced code and inline code are represented by literal
 * nodes rather than text children, so their contents remain untouched.
 */
export function remarkSoftBreaks() {
  return (tree: MarkdownAstNode) => {
    transformSoftBreaks(tree);
  };
}
