'use client';

import type { Dispatch, ReactNode, SetStateAction } from 'react';
import { useEffect, useRef, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import {
  CheckCircle2,
  ExternalLink,
  FileSearch,
  Loader2,
  RefreshCw,
  Save,
  Settings2,
  XCircle,
} from 'lucide-react';
import { useT, type AllTranslationKeys } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { contentParseService } from '@/services/content-parse.service';
import type {
  MineruParserMode,
  ParserProviderSettings,
  ParserSettingsProviderKey,
  UpsertParserSettingsRequest,
} from '@/services/types/content-parse';
import { cn } from '@/lib/utils';

const PARSER_SETTINGS_QUERY_KEY = ['content-parse', 'parser-settings'] as const;
const REDUCTO_STUDIO_URL = 'https://studio.reducto.ai';
const MINERU_TOKEN_URL = 'https://mineru.net/apiManage/token';

const REDUCTO_DEFAULTS = {
  enabled: false,
  base_url: 'https://platform.reducto.ai',
  timeout_sec: 180,
};

const MINERU_DEFAULTS = {
  enabled: false,
  mode: 'sidecar' as MineruParserMode,
  sidecar_base_url: 'http://127.0.0.1:18091',
  official_base_url: 'https://mineru.net',
  timeout_sec: 800,
  official_model_version: 'vlm',
  official_poll_interval_seconds: 3,
};

interface ReductoFormState {
  enabled: boolean;
  api_key: string;
  base_url: string;
  timeout_sec: number;
}

interface MineruFormState {
  enabled: boolean;
  mode: MineruParserMode;
  base_url: string;
  timeout_sec: number;
  official_token: string;
  official_model_version: string;
  official_poll_interval_seconds: number;
}

function initialReducto(item?: ParserProviderSettings): ReductoFormState {
  return {
    enabled: item?.enabled ?? REDUCTO_DEFAULTS.enabled,
    api_key: '',
    base_url: item?.base_url || REDUCTO_DEFAULTS.base_url,
    timeout_sec: item?.timeout_sec || REDUCTO_DEFAULTS.timeout_sec,
  };
}

function initialMineru(item?: ParserProviderSettings): MineruFormState {
  const mode = item?.mode === 'official' ? 'official' : MINERU_DEFAULTS.mode;
  return {
    enabled: item?.enabled ?? MINERU_DEFAULTS.enabled,
    mode,
    base_url:
      item?.base_url ||
      (mode === 'official' ? MINERU_DEFAULTS.official_base_url : MINERU_DEFAULTS.sidecar_base_url),
    timeout_sec: item?.timeout_sec || MINERU_DEFAULTS.timeout_sec,
    official_token: '',
    official_model_version:
      item?.official_model_version || MINERU_DEFAULTS.official_model_version,
    official_poll_interval_seconds:
      item?.official_poll_interval_seconds || MINERU_DEFAULTS.official_poll_interval_seconds,
  };
}

export default function ParserSettingsPage() {
  const t = useT();
  const router = useRouter();
  const searchParams = useSearchParams();
  const queryClient = useQueryClient();
  const reductoRef = useRef<HTMLDivElement | null>(null);
  const mineruRef = useRef<HTMLDivElement | null>(null);
  const targetProvider = searchParams.get('provider') as ParserSettingsProviderKey | null;
  const returnTo = searchParams.get('returnTo');
  const [highlight, setHighlight] = useState<ParserSettingsProviderKey | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: PARSER_SETTINGS_QUERY_KEY,
    queryFn: () => contentParseService.listParserSettings(),
  });

  const items = data?.data.items ?? [];
  const reductoSettings = items.find(item => item.provider_key === 'reducto');
  const mineruSettings = items.find(item => item.provider_key === 'mineru');

  const [reducto, setReducto] = useState<ReductoFormState>(() => initialReducto());
  const [mineru, setMineru] = useState<MineruFormState>(() => initialMineru());
  const [checkingProvider, setCheckingProvider] = useState<ParserSettingsProviderKey | null>(null);

  useEffect(() => {
    if (isLoading) return;
    setReducto(initialReducto(reductoSettings));
    setMineru(initialMineru(mineruSettings));
  }, [isLoading, reductoSettings, mineruSettings]);

  useEffect(() => {
    if (targetProvider !== 'reducto' && targetProvider !== 'mineru') return;
    const ref = targetProvider === 'reducto' ? reductoRef : mineruRef;
    ref.current?.scrollIntoView({ behavior: 'smooth', block: 'center' });
    setHighlight(targetProvider);
    const timer = window.setTimeout(() => setHighlight(null), 1600);
    return () => window.clearTimeout(timer);
  }, [targetProvider]);

  const saveMutation = useMutation({
    mutationFn: ({
      provider,
      payload,
    }: {
      provider: ParserSettingsProviderKey;
      payload: UpsertParserSettingsRequest;
    }) => contentParseService.upsertParserSettings(provider, payload),
    onSuccess: async response => {
      toast.success(
        response.data.status === 'available'
          ? t('dashboard.configuration.parserSettings.messages.savedAndValidated')
          : t('dashboard.configuration.parserSettings.messages.saved')
      );
      await queryClient.invalidateQueries({ queryKey: PARSER_SETTINGS_QUERY_KEY });
      await queryClient.invalidateQueries({ queryKey: ['content-parse', 'file-route-providers'] });
    },
    onError: error => {
      toast.error((error as { message?: string }).message || t('dashboard.configuration.parserSettings.messages.saveFailed'));
    },
  });

  const checkMutation = useMutation({
    mutationFn: (provider: ParserSettingsProviderKey) =>
      contentParseService.checkParserSettings(provider),
    onMutate: provider => {
      setCheckingProvider(provider);
    },
    onSuccess: async () => {
      toast.success(t('dashboard.configuration.parserSettings.messages.checked'));
      await queryClient.invalidateQueries({ queryKey: PARSER_SETTINGS_QUERY_KEY });
      await queryClient.invalidateQueries({ queryKey: ['content-parse', 'file-route-providers'] });
    },
    onError: async error => {
      toast.error((error as { message?: string }).message || t('dashboard.configuration.parserSettings.messages.checkFailed'));
      await queryClient.invalidateQueries({ queryKey: PARSER_SETTINGS_QUERY_KEY });
      await queryClient.invalidateQueries({ queryKey: ['content-parse', 'file-route-providers'] });
    },
    onSettled: () => {
      setCheckingProvider(null);
    },
  });

  const saveReducto = () => {
    saveMutation.mutate({
      provider: 'reducto',
      payload: {
        enabled: reducto.enabled,
        api_key: reducto.api_key.trim() || undefined,
        base_url: reducto.base_url,
        timeout_sec: reducto.timeout_sec,
      },
    });
  };

  const saveMineru = () => {
    saveMutation.mutate({
      provider: 'mineru',
      payload: {
        enabled: mineru.enabled,
        mode: mineru.mode,
        base_url: mineru.base_url,
        timeout_sec: mineru.timeout_sec,
        official_token: mineru.official_token.trim() || undefined,
        official_model_version:
          mineru.mode === 'official' ? mineru.official_model_version : undefined,
        official_poll_interval_seconds:
          mineru.mode === 'official' ? mineru.official_poll_interval_seconds : undefined,
      },
    });
  };

  const savingProvider = saveMutation.variables?.provider;
  const isSavingReducto = saveMutation.isPending && savingProvider === 'reducto';
  const isSavingMineru = saveMutation.isPending && savingProvider === 'mineru';
  const isCheckingReducto = checkMutation.isPending && checkingProvider === 'reducto';
  const isCheckingMineru = checkMutation.isPending && checkingProvider === 'mineru';

  return (
    <div className="container max-w-5xl space-y-5 py-6">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="space-y-1.5">
          <h1 className="text-2xl font-semibold tracking-tight">
            {t('dashboard.configuration.parserSettings.title')}
          </h1>
          <p className="max-w-3xl text-sm leading-6 text-muted-foreground">
            {t('dashboard.configuration.parserSettings.description')}
          </p>
        </div>
        {returnTo ? (
          <Button variant="outline" onClick={() => router.push(returnTo)}>
            {t('dashboard.configuration.parserSettings.actions.returnToReparse')}
          </Button>
        ) : null}
      </div>

      <ParserSetupGuide />

      <div className="grid gap-4">
        <div ref={reductoRef}>
          <ParserCardShell
            highlighted={highlight === 'reducto'}
            icon={<FileSearch className="h-5 w-5" />}
            title="Reducto"
            description={t('dashboard.configuration.parserSettings.reducto.description')}
            status={reductoSettings}
          >
            <div className="space-y-5">
              <EnabledRow
                enabled={reducto.enabled}
                onChange={enabled => setReducto(prev => ({ ...prev, enabled }))}
                label={t('dashboard.configuration.parserSettings.fields.enabled')}
              />
              <ProviderSetupHelp
                title={t('dashboard.configuration.parserSettings.reducto.help.title')}
                steps={[
                  t('dashboard.configuration.parserSettings.reducto.help.steps.signIn'),
                  t('dashboard.configuration.parserSettings.reducto.help.steps.createKey'),
                  t('dashboard.configuration.parserSettings.reducto.help.steps.pasteKey'),
                ]}
                actionLabel={t('dashboard.configuration.parserSettings.reducto.help.action')}
                actionHref={REDUCTO_STUDIO_URL}
              />
              <Field label={t('dashboard.configuration.parserSettings.fields.apiKey')}>
                <Input
                  type="password"
                  value={reducto.api_key}
                  placeholder={
                    reductoSettings?.api_key_configured
                      ? t('dashboard.configuration.parserSettings.placeholders.secretConfigured')
                      : t('dashboard.configuration.parserSettings.placeholders.secretRequired')
                  }
                  onChange={event => setReducto(prev => ({ ...prev, api_key: event.target.value }))}
                />
              </Field>
              <Field label={t('dashboard.configuration.parserSettings.fields.baseUrl')}>
                <Input
                  value={reducto.base_url}
                  onChange={event => setReducto(prev => ({ ...prev, base_url: event.target.value }))}
                />
                <p className="text-xs leading-5 text-muted-foreground">
                  {t('dashboard.configuration.parserSettings.hints.baseUrl')}
                </p>
              </Field>
              <Field label={t('dashboard.configuration.parserSettings.fields.timeout')}>
                <Input
                  type="number"
                  min={1}
                  value={reducto.timeout_sec}
                  onChange={event =>
                    setReducto(prev => ({ ...prev, timeout_sec: Number(event.target.value) || 1 }))
                  }
                />
              </Field>
              <SaveRow
                loading={isSavingReducto}
                checking={isCheckingReducto}
                checkDisabled={!reductoSettings?.configured}
                onSave={saveReducto}
                onCheck={() => checkMutation.mutate('reducto')}
              />
            </div>
          </ParserCardShell>
        </div>

        <div ref={mineruRef}>
          <ParserCardShell
            highlighted={highlight === 'mineru'}
            icon={<Settings2 className="h-5 w-5" />}
            title="MinerU"
            description={t('dashboard.configuration.parserSettings.mineru.description')}
            status={mineruSettings}
          >
            <div className="space-y-5">
              <EnabledRow
                enabled={mineru.enabled}
                onChange={enabled => setMineru(prev => ({ ...prev, enabled }))}
                label={t('dashboard.configuration.parserSettings.fields.enabled')}
              />
              <div className="space-y-2">
                <Label>{t('dashboard.configuration.parserSettings.fields.mode')}</Label>
                <Tabs
                  value={mineru.mode}
                  onValueChange={value =>
                    setMineru(prev => {
                      const mode = value as MineruParserMode;
                      return {
                        ...prev,
                        mode,
                        base_url:
                          mode === 'official'
                            ? MINERU_DEFAULTS.official_base_url
                            : MINERU_DEFAULTS.sidecar_base_url,
                        timeout_sec:
                          mode === 'official'
                            ? MINERU_DEFAULTS.timeout_sec
                            : MINERU_DEFAULTS.timeout_sec,
                      };
                    })
                  }
                >
                  <TabsList>
                    <TabsTrigger value="sidecar">
                      {t('dashboard.configuration.parserSettings.mineru.modes.sidecar')}
                    </TabsTrigger>
                    <TabsTrigger value="official">
                      {t('dashboard.configuration.parserSettings.mineru.modes.official')}
                    </TabsTrigger>
                  </TabsList>
                </Tabs>
              </div>
              {mineru.mode === 'official' ? (
                <OfficialMineruFields
                  value={mineru}
                  configured={Boolean(mineruSettings?.official_token_configured)}
                  onChange={setMineru}
                />
              ) : (
                <SidecarMineruFields value={mineru} onChange={setMineru} />
              )}
              <SaveRow
                loading={isSavingMineru}
                checking={isCheckingMineru}
                checkDisabled={!mineruSettings?.configured}
                onSave={saveMineru}
                onCheck={() => checkMutation.mutate('mineru')}
              />
            </div>
          </ParserCardShell>
        </div>
      </div>
    </div>
  );
}

function ParserCardShell({
  highlighted,
  icon,
  title,
  description,
  status,
  children,
}: {
  highlighted: boolean;
  icon: ReactNode;
  title: string;
  description: string;
  status?: ParserProviderSettings;
  children: ReactNode;
}) {
  return (
    <Card className={cn('transition-shadow', highlighted && 'ring-2 ring-primary shadow-lg')}>
      <CardHeader className="pb-4">
        <div className="flex items-start justify-between gap-4">
          <div className="flex min-w-0 items-start gap-3">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
              {icon}
            </div>
            <div className="min-w-0">
              <CardTitle className="text-lg">{title}</CardTitle>
              <CardDescription className="mt-1 leading-5">{description}</CardDescription>
            </div>
          </div>
          <StatusBadge status={status} />
        </div>
      </CardHeader>
      <CardContent>
        {status?.validation_message && status.status === 'failed' ? (
          <div className="mb-4 rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm leading-5 text-destructive">
            {status.validation_message}
          </div>
        ) : null}
        {children}
      </CardContent>
    </Card>
  );
}

function ParserSetupGuide() {
  const t = useT();
  return (
    <section className="rounded-lg border border-primary/20 bg-primary/5 px-4 py-3">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div className="space-y-1">
          <div className="text-sm font-semibold">
            {t('dashboard.configuration.parserSettings.guide.title')}
          </div>
          <p className="max-w-3xl text-sm leading-6 text-muted-foreground">
            {t('dashboard.configuration.parserSettings.guide.description')}
          </p>
          <div className="flex flex-wrap gap-x-6 gap-y-1 text-sm font-medium leading-6 text-foreground/80">
            <span>{t('dashboard.configuration.parserSettings.guide.reductoRecommendation')}</span>
            <span>{t('dashboard.configuration.parserSettings.guide.mineruRecommendation')}</span>
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap gap-2">
          <Button variant="outline" size="sm" asChild>
            <a href={REDUCTO_STUDIO_URL} target="_blank" rel="noreferrer" className="gap-2">
              {t('dashboard.configuration.parserSettings.guide.openReducto')}
              <ExternalLink className="h-3.5 w-3.5" />
            </a>
          </Button>
          <Button variant="outline" size="sm" asChild>
            <a href={MINERU_TOKEN_URL} target="_blank" rel="noreferrer" className="gap-2">
              {t('dashboard.configuration.parserSettings.guide.openMineru')}
              <ExternalLink className="h-3.5 w-3.5" />
            </a>
          </Button>
        </div>
      </div>
    </section>
  );
}

function ProviderSetupHelp({
  title,
  steps,
  actionLabel,
  actionHref,
}: {
  title: string;
  steps: string[];
  actionLabel: string;
  actionHref: string;
}) {
  return (
    <div className="rounded-lg border bg-muted/20 px-3 py-3">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="space-y-2">
          <div className="text-sm font-medium">{title}</div>
          <ol className="list-decimal space-y-1 pl-5 text-xs leading-5 text-muted-foreground">
            {steps.map(step => (
              <li key={step}>{step}</li>
            ))}
          </ol>
        </div>
        <Button variant="outline" size="sm" asChild className="shrink-0">
          <a href={actionHref} target="_blank" rel="noreferrer" className="gap-2">
            {actionLabel}
            <ExternalLink className="h-3.5 w-3.5" />
          </a>
        </Button>
      </div>
    </div>
  );
}

function StatusBadge({ status }: { status?: ParserProviderSettings }) {
  const t = useT();
  const value = status?.status ?? 'not_configured';
  const ready = value === 'available';
  return (
    <Badge variant={ready ? 'success' : value === 'disabled' ? 'secondary' : 'outline'} className="gap-1">
      {ready ? <CheckCircle2 className="h-3 w-3" /> : <XCircle className="h-3 w-3" />}
      {t(`dashboard.configuration.parserSettings.status.${value}` as AllTranslationKeys)}
    </Badge>
  );
}

function EnabledRow({
  enabled,
  onChange,
  label,
}: {
  enabled: boolean;
  onChange: (enabled: boolean) => void;
  label: string;
}) {
  return (
    <div className="flex items-center justify-between rounded-lg border bg-muted/20 px-3 py-2">
      <Label className="text-sm font-medium">{label}</Label>
      <Switch checked={enabled} onCheckedChange={onChange} />
    </div>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      {children}
    </div>
  );
}

function SaveRow({
  loading,
  checking,
  checkDisabled,
  onSave,
  onCheck,
}: {
  loading: boolean;
  checking: boolean;
  checkDisabled: boolean;
  onSave: () => void;
  onCheck: () => void;
}) {
  const t = useT();
  return (
    <div className="flex flex-wrap justify-end gap-2 border-t pt-4">
      <Button
        type="button"
        variant="outline"
        onClick={onCheck}
        disabled={loading || checking || checkDisabled}
        className="gap-2"
      >
        {checking ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
        {t('dashboard.configuration.parserSettings.actions.check')}
      </Button>
      <Button onClick={onSave} disabled={loading} className="gap-2">
        {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
        {t('dashboard.configuration.parserSettings.actions.save')}
      </Button>
    </div>
  );
}

function SidecarMineruFields({
  value,
  onChange,
}: {
  value: MineruFormState;
  onChange: Dispatch<SetStateAction<MineruFormState>>;
}) {
  const t = useT();
  return (
    <>
      <Field label={t('dashboard.configuration.parserSettings.fields.apiUrl')}>
        <Input
          value={value.base_url}
          onChange={event => onChange(prev => ({ ...prev, base_url: event.target.value }))}
        />
        <p className="text-xs leading-5 text-muted-foreground">
          {t('dashboard.configuration.parserSettings.hints.baseUrl')}
        </p>
      </Field>
      <Field label={t('dashboard.configuration.parserSettings.fields.timeout')}>
        <Input
          type="number"
          min={1}
          value={value.timeout_sec}
          onChange={event => onChange(prev => ({ ...prev, timeout_sec: Number(event.target.value) || 1 }))}
        />
      </Field>
    </>
  );
}

function OfficialMineruFields({
  value,
  configured,
  onChange,
}: {
  value: MineruFormState;
  configured: boolean;
  onChange: Dispatch<SetStateAction<MineruFormState>>;
}) {
  const t = useT();
  return (
    <>
      <ProviderSetupHelp
        title={t('dashboard.configuration.parserSettings.mineru.help.title')}
        steps={[
          t('dashboard.configuration.parserSettings.mineru.help.steps.signIn'),
          t('dashboard.configuration.parserSettings.mineru.help.steps.createToken'),
          t('dashboard.configuration.parserSettings.mineru.help.steps.pasteToken'),
        ]}
        actionLabel={t('dashboard.configuration.parserSettings.mineru.help.action')}
        actionHref={MINERU_TOKEN_URL}
      />
      <Field label={t('dashboard.configuration.parserSettings.fields.officialToken')}>
        <Input
          type="password"
          value={value.official_token}
          placeholder={
            configured
              ? t('dashboard.configuration.parserSettings.placeholders.secretConfigured')
              : t('dashboard.configuration.parserSettings.placeholders.secretRequired')
          }
          onChange={event => onChange(prev => ({ ...prev, official_token: event.target.value }))}
        />
      </Field>
      <Field label={t('dashboard.configuration.parserSettings.fields.baseUrl')}>
        <Input
          value={value.base_url}
          onChange={event => onChange(prev => ({ ...prev, base_url: event.target.value }))}
        />
        <p className="text-xs leading-5 text-muted-foreground">
          {t('dashboard.configuration.parserSettings.hints.baseUrl')}
        </p>
      </Field>
      <div className="grid gap-4 md:grid-cols-3">
        <Field label={t('dashboard.configuration.parserSettings.fields.modelVersion')}>
          <Input
            value={value.official_model_version}
            onChange={event =>
              onChange(prev => ({ ...prev, official_model_version: event.target.value }))
            }
          />
        </Field>
        <Field label={t('dashboard.configuration.parserSettings.fields.pollInterval')}>
          <Input
            type="number"
            min={1}
            value={value.official_poll_interval_seconds}
            onChange={event =>
              onChange(prev => ({
                ...prev,
                official_poll_interval_seconds: Number(event.target.value) || 1,
              }))
            }
          />
        </Field>
        <Field label={t('dashboard.configuration.parserSettings.fields.timeout')}>
          <Input
            type="number"
            min={1}
            value={value.timeout_sec}
            onChange={event =>
              onChange(prev => ({ ...prev, timeout_sec: Number(event.target.value) || 1 }))
            }
          />
        </Field>
      </div>
    </>
  );
}
