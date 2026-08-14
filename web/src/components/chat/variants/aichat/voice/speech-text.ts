export function toAgentSpeechText(markdown: string): string {
  return markdown
    .replace(/\r\n?/g, '\n')
    .replace(/^\s*```[^\n]*$/gm, '')
    .replace(/!\[([^\]]*)\]\(([^)]+)\)/g, (_match, label: string, url: string) =>
      [label.trim(), url.trim()].filter(Boolean).join(' ')
    )
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, (_match, label: string, url: string) =>
      [label.trim(), url.trim()].filter(Boolean).join(' ')
    )
    .replace(/`([^`]+)`/g, '$1')
    .replace(/^\s{0,3}#{1,6}\s+/gm, '')
    .replace(/^\s*(?:[-+*]|\d+[.)])\s+/gm, '')
    .replace(/^\s*>\s?/gm, '')
    .replace(/^\s*\|?(?:\s*:?-{3,}:?\s*\|)+\s*$/gm, '')
    .replace(/\|/g, ' ')
    .replace(/\*\*|__|~~/g, '')
    .replace(/(^|\s)[*_]([^*_\n]+)[*_](?=\s|$|[.,!?;:])/g, '$1$2')
    .split('\n')
    .map(line => line.replace(/[ \t]+/g, ' ').trim())
    .filter(Boolean)
    .join('\n')
    .trim();
}
