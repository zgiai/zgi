'use client';

import { useState } from 'react';
import { BookOpen } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

const samplePayload = `{
  "id": "4f0c…",                       // delivery id, use it to drop duplicates
  "event": "report.created",
  "created_at": "2026-10-22T09:00:05Z",
  "data": {
    "notification_id": "…",
    "digital_human_id": "…",
    "task_id": "…",
    "report_id": "…",                  // read it with GET /reports/{report_id}
    "outcome": "done",
    "client_reference": "crm-ticket-42", // the value you gave when creating the task
    "title": "Weekly summary",
    "summary": "…"
  }
}`;

const verifiers = {
  node: `import crypto from 'node:crypto';

export function verify(rawBody, headers, secret) {
  const timestamp = headers['x-zgi-timestamp'];
  if (Math.abs(Date.now() / 1000 - Number(timestamp)) > 300) return false;
  const expected = 'sha256=' + crypto
    .createHmac('sha256', secret)
    .update(\`\${timestamp}.\${rawBody}\`)
    .digest('hex');
  const given = headers['x-zgi-signature'] ?? '';
  return given.length === expected.length &&
    crypto.timingSafeEqual(Buffer.from(given), Buffer.from(expected));
}`,
  python: `import hashlib, hmac, time

def verify(raw_body: bytes, headers, secret: str) -> bool:
    timestamp = headers["X-ZGI-Timestamp"]
    if abs(time.time() - int(timestamp)) > 300:
        return False
    digest = hmac.new(secret.encode(), f"{timestamp}.".encode() + raw_body, hashlib.sha256)
    expected = "sha256=" + digest.hexdigest()
    return hmac.compare_digest(headers.get("X-ZGI-Signature", ""), expected)`,
  go: `func verify(rawBody []byte, h http.Header, secret string) bool {
	ts, err := strconv.ParseInt(h.Get("X-ZGI-Timestamp"), 10, 64)
	if err != nil || math.Abs(float64(time.Now().Unix()-ts)) > 300 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.", ts)
	mac.Write(rawBody)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(h.Get("X-ZGI-Signature")), []byte(expected))
}`,
} as const;

type Language = keyof typeof verifiers;

/** How an integrator receives and checks the DigitalHuman's API callbacks. */
export function DigitalHumanApiCallbackGuide() {
  const t = useT('digitalHumans');
  const [open, setOpen] = useState(false);
  const [language, setLanguage] = useState<Language>('node');
  const events = [
    ['report.created', 'callbackGuide.events.report_created'],
    ['task.failed', 'callbackGuide.events.task_failed'],
    ['task.approval_requested', 'callbackGuide.events.task_approval_requested'],
    ['notification.test', 'callbackGuide.events.notification_test'],
  ] as const;
  return (
    <>
      <Button type="button" size="sm" variant="outline" onClick={() => setOpen(true)}>
        <BookOpen className="size-4" />
        {t('callbackGuide.open')}
      </Button>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent side="right" className="w-[min(640px,100vw)] max-w-none overflow-y-auto sm:max-w-none">
          <SheetHeader className="text-left">
            <SheetTitle>{t('callbackGuide.title')}</SheetTitle>
            <SheetDescription>{t('callbackGuide.description')}</SheetDescription>
          </SheetHeader>
          <div className="mt-5 space-y-6 text-sm">
            <section>
              <h3 className="font-semibold">{t('callbackGuide.eventsTitle')}</h3>
              <ul className="mt-2 divide-y rounded-lg border">
                {events.map(([event, label]) => (
                  <li key={event} className="flex gap-3 px-3 py-2">
                    <code className="w-48 shrink-0 text-xs">{event}</code>
                    <span className="text-muted-foreground">{t(label)}</span>
                  </li>
                ))}
              </ul>
            </section>
            <section>
              <h3 className="font-semibold">{t('callbackGuide.payloadTitle')}</h3>
              <p className="mt-1 text-muted-foreground">{t('callbackGuide.payloadDescription')}</p>
              <pre className="mt-2 overflow-x-auto rounded-lg bg-muted p-3 font-mono text-xs leading-5">{samplePayload}</pre>
            </section>
            <section>
              <h3 className="font-semibold">{t('callbackGuide.verifyTitle')}</h3>
              <p className="mt-1 text-muted-foreground">{t('callbackGuide.verifyDescription')}</p>
              <div className="mt-2 flex gap-1">
                {(Object.keys(verifiers) as Language[]).map(item => (
                  <Button
                    key={item}
                    type="button"
                    size="sm"
                    variant={language === item ? 'default' : 'outline'}
                    className={cn('h-7 px-3 text-xs')}
                    onClick={() => setLanguage(item)}
                  >
                    {item === 'node' ? 'Node.js' : item === 'python' ? 'Python' : 'Go'}
                  </Button>
                ))}
              </div>
              <pre className="mt-2 overflow-x-auto rounded-lg bg-muted p-3 font-mono text-xs leading-5">{verifiers[language]}</pre>
            </section>
            <section>
              <h3 className="font-semibold">{t('callbackGuide.deliveryTitle')}</h3>
              <ul className="mt-2 list-disc space-y-1 pl-5 text-muted-foreground">
                <li>{t('callbackGuide.delivery.respond')}</li>
                <li>{t('callbackGuide.delivery.retry')}</li>
                <li>{t('callbackGuide.delivery.dedupe')}</li>
                <li>{t('callbackGuide.delivery.route')}</li>
                <li>{t('callbackGuide.delivery.public')}</li>
              </ul>
            </section>
          </div>
        </SheetContent>
      </Sheet>
    </>
  );
}
