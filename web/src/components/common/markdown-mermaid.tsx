import React from 'react';
import { cn } from '@/lib/utils';

interface MarkdownMermaidProps {
  chart: string;
  cacheKey: string;
  className?: string;
  fallbackClassName?: string;
}

let mermaidInitialized = false;

type MermaidCacheEntry =
  | { status: 'success'; svg: string }
  | { status: 'failed' };

const MERMAID_CACHE_LIMIT = 120;
const mermaidRenderCache = new Map<string, MermaidCacheEntry>();

function sanitizeMermaidId(value: string): string {
  const cleaned = value.replace(/[^a-zA-Z0-9_-]/g, '-').replace(/-+/g, '-');
  return cleaned || 'diagram';
}

function readMermaidCache(key: string): MermaidCacheEntry | undefined {
  const entry = mermaidRenderCache.get(key);
  if (!entry) return undefined;
  mermaidRenderCache.delete(key);
  mermaidRenderCache.set(key, entry);
  return entry;
}

function writeMermaidCache(key: string, entry: MermaidCacheEntry) {
  if (mermaidRenderCache.has(key)) {
    mermaidRenderCache.delete(key);
  }
  mermaidRenderCache.set(key, entry);
  while (mermaidRenderCache.size > MERMAID_CACHE_LIMIT) {
    const oldest = mermaidRenderCache.keys().next().value;
    if (!oldest) break;
    mermaidRenderCache.delete(oldest);
  }
}

function stateFromCache(entry: MermaidCacheEntry | undefined) {
  if (!entry) return { svg: '', failed: false, loading: true };
  if (entry.status === 'success') return { svg: entry.svg, failed: false, loading: false };
  return { svg: '', failed: true, loading: false };
}

export function MarkdownMermaid({
  chart,
  cacheKey,
  className,
  fallbackClassName,
}: MarkdownMermaidProps) {
  const id = React.useId();
  const diagramId = React.useMemo(
    () => `md-mermaid-${sanitizeMermaidId(cacheKey || id)}`,
    [cacheKey, id]
  );
  const [renderState, setRenderState] = React.useState(() =>
    stateFromCache(readMermaidCache(cacheKey))
  );

  React.useEffect(() => {
    let active = true;

    const render = async () => {
      try {
        const mermaidModule = await import('mermaid');
        const mermaid = mermaidModule.default;

        if (!mermaidInitialized) {
          mermaid.initialize({
            startOnLoad: false,
            securityLevel: 'strict',
            suppressErrorRendering: true,
            fontFamily: 'inherit',
          });
          mermaidInitialized = true;
        }

        const { svg: output } = await mermaid.render(diagramId, chart);
        writeMermaidCache(cacheKey, { status: 'success', svg: output });
        if (!active) return;
        setRenderState({ svg: output, failed: false, loading: false });
      } catch {
        writeMermaidCache(cacheKey, { status: 'failed' });
        if (!active) return;
        setRenderState({ svg: '', failed: true, loading: false });
      }
    };

    if (chart.trim().length === 0) {
      setRenderState({ svg: '', failed: false, loading: false });
      return () => {
        active = false;
      };
    }

    const cached = readMermaidCache(cacheKey);
    if (cached) {
      setRenderState(stateFromCache(cached));
      return () => {
        active = false;
      };
    }

    setRenderState({ svg: '', failed: false, loading: true });
    void render();

    return () => {
      active = false;
    };
  }, [cacheKey, chart, diagramId]);

  if (renderState.failed) {
    return (
      <div className={cn('md-mermaid-fallback', fallbackClassName)}>
        <pre className="overflow-auto text-xs">
          <code className="language-mermaid">{chart}</code>
        </pre>
      </div>
    );
  }

  return (
    <div className={cn('md-mermaid-wrapper', className)}>
      {renderState.svg ? (
        <div className="md-mermaid-graph" dangerouslySetInnerHTML={{ __html: renderState.svg }} />
      ) : (
        <div className={cn('md-mermaid-loading', renderState.loading ? 'animate-pulse' : '')} />
      )}
    </div>
  );
}
