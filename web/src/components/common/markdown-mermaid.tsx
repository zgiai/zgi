import React from 'react';
import { cn } from '@/lib/utils';

interface MarkdownMermaidProps {
  chart: string;
  className?: string;
  fallbackClassName?: string;
}

let mermaidInitialized = false;

export function MarkdownMermaid({ chart, className, fallbackClassName }: MarkdownMermaidProps) {
  const id = React.useId();
  const [svg, setSvg] = React.useState('');
  const [failed, setFailed] = React.useState(false);
  const diagramId = React.useMemo(() => `md-mermaid-${id.replace(/[^a-zA-Z0-9_-]/g, '')}`, [id]);

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
        if (!active) return;
        setSvg(output);
        setFailed(false);
      } catch {
        if (!active) return;
        setSvg('');
        setFailed(true);
      }
    };

    if (chart.trim().length === 0) {
      setSvg('');
      setFailed(false);
      return () => {
        active = false;
      };
    }

    setSvg('');
    setFailed(false);
    void render();

    return () => {
      active = false;
    };
  }, [chart, diagramId]);

  if (failed) {
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
      {svg ? (
        <div className="md-mermaid-graph" dangerouslySetInnerHTML={{ __html: svg }} />
      ) : (
        <div className="md-mermaid-loading animate-pulse" />
      )}
    </div>
  );
}
