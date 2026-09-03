'use client';

import { Activity, KeyRound, ShieldCheck } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useT } from '@/i18n';

interface ConnectionMaintenanceCardProps {
  canReconnect: boolean;
  diagnosing: boolean;
  onReconnect: () => void;
  onDiagnose: () => void;
  isApp?: boolean;
  isMessaging?: boolean;
  onConfigure?: () => void;
}

export function ConnectionMaintenanceCard({
  canReconnect,
  diagnosing,
  onReconnect,
  onDiagnose,
  isApp = false,
  isMessaging = false,
  onConfigure,
}: ConnectionMaintenanceCardProps) {
  const t = useT('connectors');
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('maintenance.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-0">
        <div className="flex flex-col gap-4 py-4 first:pt-0 sm:flex-row sm:items-center">
          <div className="flex min-w-0 flex-1 gap-3">
            <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <KeyRound aria-hidden="true" className="size-5" />
            </div>
            <div className="min-w-0">
              <p className="font-medium">
                {t(
                  isMessaging
                    ? 'messaging.reauthorizeTitle'
                    : isApp
                      ? 'app.updateTitle'
                      : 'maintenance.updateKeyTitle'
                )}
              </p>
              <p className="mt-1 text-sm leading-5 text-muted-foreground">
                {t(
                  isMessaging
                    ? 'messaging.reauthorizeHint'
                    : isApp
                      ? 'app.updateHint'
                      : 'maintenance.updateKeyDescription'
                )}
              </p>
            </div>
          </div>
          <Button
            variant="outline"
            className="w-full sm:w-44"
            disabled={!canReconnect}
            onClick={onReconnect}
          >
            {t('maintenance.updateAction')}
          </Button>
        </div>

        <div className="flex flex-col gap-4 border-t py-4 sm:flex-row sm:items-center">
          <div className="flex min-w-0 flex-1 gap-3">
            <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <Activity aria-hidden="true" className="size-5" />
            </div>
            <div className="min-w-0">
              <p className="font-medium">{t('maintenance.diagnoseTitle')}</p>
              <p className="mt-1 text-sm leading-5 text-muted-foreground">
                {t('maintenance.diagnoseDescription')}
              </p>
            </div>
          </div>
          <Button
            variant="outline"
            className="w-full sm:w-44"
            loading={diagnosing}
            onClick={onDiagnose}
          >
            {diagnosing ? t('maintenance.diagnosing') : t('maintenance.diagnoseAction')}
          </Button>
        </div>

        {onConfigure && (
          <div className="flex flex-col gap-4 border-t py-4 sm:flex-row sm:items-center">
            <div className="min-w-0 flex-1">
              <p className="font-medium">{t('app.repairTitle')}</p>
              <p className="mt-1 text-sm leading-5 text-muted-foreground">{t('app.repairHint')}</p>
            </div>
            <Button variant="outline" className="w-full sm:w-44" onClick={onConfigure}>
              {t('app.repair')}
            </Button>
          </div>
        )}
        <div className="flex items-start gap-2 rounded-lg border border-primary/15 bg-primary/5 px-3 py-2.5 text-xs leading-5 text-primary">
          <ShieldCheck aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
          <span>{t(isApp ? 'app.secretNotice' : 'maintenance.securityNotice')}</span>
        </div>
      </CardContent>
    </Card>
  );
}
