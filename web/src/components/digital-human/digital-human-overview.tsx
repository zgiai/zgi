'use client';

import Link from 'next/link';
import {
  ArrowRight,
  BriefcaseBusiness,
  CheckCircle2,
  Circle,
  MessageSquare,
  PlayCircle,
  Wrench,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { DigitalHumanPublishPanel } from '@/components/digital-human/digital-human-configuration';
import {
  useDigitalHuman,
  useDigitalHumanDraftBindings,
  useDigitalHumanDraftRoles,
  useDigitalHumanRuns,
} from '@/hooks/digital-human/use-digital-humans';
import { useT } from '@/i18n';

export function DigitalHumanOverview({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const detailQuery = useDigitalHuman(id);
  const rolesQuery = useDigitalHumanDraftRoles(id);
  const bindingsQuery = useDigitalHumanDraftBindings(id);
  const runsQuery = useDigitalHumanRuns(id);
  const detail = detailQuery.data?.data;
  if (detailQuery.isLoading || !detail) {
    return (
      <div className="mx-auto max-w-6xl p-8">
        <div className="h-72 animate-pulse rounded-xl bg-muted/40" />
      </div>
    );
  }

  const roles = rolesQuery.data?.data?.items ?? [];
  const bindings = bindingsQuery.data?.data?.items ?? [];
  const runs = runsQuery.data?.data?.items ?? [];
  const steps = [
    {
      ready: Boolean(detail.definition.name.trim() && detail.draft.persona.trim()),
      title: t('overview.steps.identity'),
      description: t('overview.steps.identityDescription'),
      href: `/console/digital-humans/${id}/settings`,
    },
    {
      ready: Boolean(detail.draft.rules.length && detail.draft.default_model.trim()),
      title: t('overview.steps.behavior'),
      description: t('overview.steps.behaviorDescription'),
      href: `/console/digital-humans/${id}/settings`,
    },
    {
      ready: roles.length > 0,
      title: t('overview.steps.role'),
      description: t('overview.steps.roleDescription'),
      href: `/console/digital-humans/${id}/capabilities`,
    },
    {
      ready: bindings.length > 0,
      title: t('overview.steps.resources'),
      description: t('overview.steps.resourcesDescription'),
      href: `/console/digital-humans/${id}/capabilities`,
    },
    {
      ready: Boolean(detail.published),
      title: t('overview.steps.publish'),
      description: t('overview.steps.publishDescription'),
      href: '#publish',
    },
  ];
  const completed = steps.filter(step => step.ready).length;

  return (
    <div className="mx-auto max-w-6xl space-y-6 px-4 py-6 md:px-8">
      <div className="flex flex-col gap-4 rounded-2xl border bg-gradient-to-br from-background to-muted/30 p-6 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <p className="text-sm font-medium text-primary">{t('overview.eyebrow')}</p>
          <h2 className="mt-1 text-2xl font-semibold tracking-tight">{t('overview.title')}</h2>
          <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
            {t('overview.description')}
          </p>
        </div>
        <Button asChild variant={detail.published ? 'default' : 'outline'}>
          <Link href={`/console/digital-humans/${id}/chat`}>
            <MessageSquare className="mr-2 size-4" />
            {t('overview.test')}
          </Link>
        </Button>
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        <SummaryCard
          icon={BriefcaseBusiness}
          label={t('overview.summary.roles')}
          value={roles.length}
        />
        <SummaryCard
          icon={Wrench}
          label={t('overview.summary.resources')}
          value={bindings.length}
        />
        <SummaryCard icon={PlayCircle} label={t('overview.summary.runs')} value={runs.length} />
      </div>

      <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1.25fr)_minmax(320px,0.75fr)]">
        <Card>
          <CardHeader>
            <div className="flex items-start justify-between gap-4">
              <div>
                <CardTitle>{t('overview.setupTitle')}</CardTitle>
                <CardDescription className="mt-1">
                  {t('overview.setupProgress', { completed, total: steps.length })}
                </CardDescription>
              </div>
              <span className="rounded-full bg-muted px-3 py-1 text-sm font-medium">
                {completed}/{steps.length}
              </span>
            </div>
          </CardHeader>
          <CardContent className="space-y-2">
            {steps.map(step => (
              <Link
                key={step.title}
                href={step.href}
                className="group flex min-h-16 items-center gap-3 rounded-xl border p-3 transition-colors hover:bg-muted/40"
              >
                {step.ready ? (
                  <CheckCircle2 className="size-5 shrink-0 text-emerald-600" />
                ) : (
                  <Circle className="size-5 shrink-0 text-muted-foreground" />
                )}
                <div className="min-w-0 flex-1">
                  <div className="text-sm font-medium">{step.title}</div>
                  <p className="mt-0.5 text-xs text-muted-foreground">{step.description}</p>
                </div>
                <ArrowRight className="size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
              </Link>
            ))}
          </CardContent>
        </Card>
        <DigitalHumanPublishPanel id={id} />
      </div>
    </div>
  );
}

function SummaryCard({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof BriefcaseBusiness;
  label: string;
  value: number;
}) {
  return (
    <Card>
      <CardContent className="flex items-center gap-3 p-4">
        <div className="flex size-10 items-center justify-center rounded-xl bg-primary/10 text-primary">
          <Icon className="size-5" />
        </div>
        <div>
          <div className="text-2xl font-semibold tabular-nums">{value}</div>
          <div className="text-xs text-muted-foreground">{label}</div>
        </div>
      </CardContent>
    </Card>
  );
}
