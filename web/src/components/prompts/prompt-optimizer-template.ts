export function extractPromptVariables(rawPrompt: string): string[] {
  const patterns = [
    /<zgi:(knowledge|skill)\b[^>]*>[\s\S]*?<\/zgi:\1>/g,
    /\{\{#[^{}]+#\}\}/g,
    /\{\{[^{}]+\}\}/g,
    /\$\{[^{}]+\}/g,
  ];
  const matches = new Set<string>();

  for (const pattern of patterns) {
    const found = rawPrompt.match(pattern);
    if (!found) continue;
    for (const item of found) {
      matches.add(item);
    }
  }

  return Array.from(matches);
}
