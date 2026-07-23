'use client';

import { useEffect } from 'react';
import { captureError } from '@/lib/observability';

export default function GlobalError({ error }: { error: Error & { digest?: string } }) {
  useEffect(() => {
    captureError(error, 'ui.application.render_failed');
  }, [error]);

  return (
    <html>
      <body style={{ margin: 0, fontFamily: 'system-ui, sans-serif' }}>
        <main
          style={{
            alignItems: 'center',
            background: '#f7f7f7',
            color: '#1f2937',
            display: 'flex',
            minHeight: '100vh',
            justifyContent: 'center',
            padding: 24,
          }}
        >
          <section
            style={{
              background: '#fff',
              border: '1px solid #e5e7eb',
              borderRadius: 12,
              boxShadow: '0 10px 30px rgba(15, 23, 42, 0.08)',
              maxWidth: 640,
              padding: 24,
              width: '100%',
            }}
          >
            <div style={{ color: '#dc2626', fontSize: 14, fontWeight: 700, marginBottom: 8 }}>
              Application error
            </div>
            <h1 style={{ fontSize: 24, lineHeight: 1.2, margin: 0 }}>页面加载失败</h1>
            <p style={{ color: '#6b7280', lineHeight: 1.6, marginTop: 12 }}>
              页面初始化时发生错误。你可以刷新页面，或返回控制台后重新进入当前资源。
            </p>
            <pre
              style={{
                background: '#f3f4f6',
                borderRadius: 8,
                color: '#4b5563',
                fontSize: 12,
                lineHeight: 1.5,
                marginTop: 16,
                maxHeight: 120,
                overflow: 'auto',
                padding: 12,
                whiteSpace: 'pre-wrap',
              }}
            >
              {error.message || error.digest || 'Unknown error'}
            </pre>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginTop: 20 }}>
              <button
                onClick={() => window.location.reload()}
                style={{
                  background: '#111827',
                  border: 0,
                  borderRadius: 8,
                  color: '#fff',
                  cursor: 'pointer',
                  fontSize: 14,
                  fontWeight: 600,
                  padding: '10px 14px',
                }}
              >
                刷新页面
              </button>
              <button
                onClick={() => (window.location.href = '/console')}
                style={{
                  background: '#fff',
                  border: '1px solid #d1d5db',
                  borderRadius: 8,
                  color: '#111827',
                  cursor: 'pointer',
                  fontSize: 14,
                  fontWeight: 600,
                  padding: '10px 14px',
                }}
              >
                返回控制台
              </button>
            </div>
          </section>
        </main>
      </body>
    </html>
  );
}
