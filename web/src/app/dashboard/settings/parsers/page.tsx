'use client';

import type { Dispatch, ReactNode, SetStateAction } from 'react';
import { useEffect, useRef, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { CheckCircle2, FileSearch, Loader2, Save, Settings2, XCircle } from 'lucide-react';
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
    onSuccess: async () => {
      toast.success(t('dashboard.configuration.parserSettings.messages.saved'));
      await queryClient.invalidateQueries({ queryKey: PARSER_SETTINGS_QUERY_KEY });
      await queryClient.invalidateQueries({ queryKey: ['content-parse', 'file-route-providers'] });
    },
    onError: error => {
      toast.error((error as { message?: string }).message || t('dashboard.configuration.parserSettings.messages.saveFailed'));
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
              <SaveRow loading={isSavingReducto} onSave={saveReducto} />
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
              <SaveRow loading={isSavingMineru} onSave={saveMineru} />
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
      <CardContent>{children}</CardContent>
    </Card>
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

function SaveRow({ loading, onSave }: { loading: boolean; onSave: () => void }) {
  const t = useT();
  return (
    <div className="flex justify-end border-t pt-4">
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
