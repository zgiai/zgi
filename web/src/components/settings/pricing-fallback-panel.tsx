'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  ChevronDown,
  ChevronUp,
  ImageIcon,
  Loader2,
  RefreshCw,
  RotateCcw,
  Save,
  Type,
} from 'lucide-react';
import { toast } from 'sonner';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Switch } from '@/components/ui/switch';
import { useT, type SettingsKey } from '@/i18n';
import modelService from '@/services/model.service';
import type {
  PricingFallbackConfig,
  PricingFallbackOperation,
  PricingFallbackRule,
  PricingFallbackSource,
} from '@/services/types/model';

const TOKEN_OPERATIONS: PricingFallbackOperation[] = ['chat', 'embedding', 'rerank'];

const OPERATION_LABEL_KEYS: Record<PricingFallbackOperation, SettingsKey> = {
  chat: 'settings.pricingFallback.operationChat',
  embedding: 'settings.pricingFallback.operationEmbedding',
  rerank: 'settings.pricingFallback.operationRerank',
  image_generation: 'settings.pricingFallback.operationImage',
};

const SOURCE_LABEL_KEYS: Record<PricingFallbackSource, SettingsKey> = {
  upstream_model_price: 'settings.pricingFallback.sourceUpstream',
  admin_fallback: 'settings.pricingFallback.sourceAdmin',
  code_default_fallback: 'settings.pricingFallback.sourceCode',
};

interface TokenRuleGroup {
  key: string;
  operation: PricingFallbackOperation;
  provider: string;
  model: string;
  input?: PricingFallbackRule;
  output?: PricingFallbackRule;
}

function cleanText(value: string | undefined) {
  return value?.trim() ?? '';
}

function matchText(value: string | undefined) {
  const v = cleanText(value);
  return v || '*';
}

function ruleIdentityPart(value: string | undefined) {
  return matchText(value).replace(/[^a-zA-Z0-9_.-]+/g, '_');
}

function sameRuleTarget(a: PricingFallbackRule, b: PricingFallbackRule) {
  return (
    a.operation === b.operation &&
    a.meter === b.meter &&
    matchText(a.provider) === matchText(b.provider) &&
    matchText(a.model) === matchText(b.model) &&
    matchText(a.size) === matchText(b.size) &&
    matchText(a.quality) === matchText(b.quality) &&
    matchText(a.style) === matchText(b.style)
  );
}

function adminRuleIDFrom(rule: PricingFallbackRule) {
  const kind = rule.meter === 'image' ? 'image' : 'token';
  return [
    'admin',
    kind,
    rule.operation,
    rule.meter,
    ruleIdentityPart(rule.provider),
    ruleIdentityPart(rule.model),
    ruleIdentityPart(rule.size),
    ruleIdentityPart(rule.quality),
    ruleIdentityPart(rule.style),
  ].join('.');
}

function mergeEditableRule(baseRule: PricingFallbackRule, overrideRule?: PricingFallbackRule) {
  if (!overrideRule) return baseRule;
  return {
    ...baseRule,
    ...overrideRule,
    pricing_source: 'admin_fallback' as PricingFallbackSource,
  };
}

function buildOverrideRule(
  baseRule: PricingFallbackRule,
  patch: Partial<PricingFallbackRule>,
  existingRule?: PricingFallbackRule
) {
  const id =
    baseRule.pricing_source === 'code_default_fallback'
      ? baseRule.id
      : existingRule?.id || baseRule.id || adminRuleIDFrom(baseRule);
  const nextRule: PricingFallbackRule = {
    ...baseRule,
    ...existingRule,
    ...patch,
    id,
    enabled: true,
  };
  delete nextRule.pricing_source;
  return nextRule;
}

function displayPattern(value: string | undefined, allLabel: string) {
  const v = cleanText(value);
  return !v || v === '*' ? allLabel : v;
}

function normalizeRuleForSave(rule: PricingFallbackRule): PricingFallbackRule {
  const id = cleanText(rule.id);
  const enabled = rule.enabled ?? true;
  const provider = cleanText(rule.provider);
  const model = cleanText(rule.model);

  if (rule.meter === 'image') {
    const size = cleanText(rule.size);
    const quality = cleanText(rule.quality);
    const style = cleanText(rule.style);
    const normalized: PricingFallbackRule = {
      id,
      enabled,
      operation: 'image_generation',
      meter: 'image',
      unit: 'credits_per_image',
      credits: Number(rule.credits ?? 0),
    };
    if (provider) normalized.provider = provider;
    if (model) normalized.model = model;
    if (size) normalized.size = size;
    if (quality) normalized.quality = quality;
    if (style) normalized.style = style;
    return normalized;
  }

  const normalized: PricingFallbackRule = {
    id,
    enabled,
    operation: rule.operation,
    meter: rule.meter,
    unit: 'usd_per_1m_tokens',
    price_usd_per_1m_tokens: cleanText(rule.price_usd_per_1m_tokens),
  };
  if (provider) normalized.provider = provider;
  if (model) normalized.model = model;
  return normalized;
}

function isValidNonNegativeNumber(value: string | undefined) {
  if (!value || value.trim() === '') return false;
  const n = Number(value);
  return Number.isFinite(n) && n >= 0;
}

function validateRules(rules: PricingFallbackRule[], t: (key: SettingsKey) => string) {
  const seen = new Set<string>();

  for (const rule of rules) {
    const id = cleanText(rule.id);
    if (!id) return t('settings.pricingFallback.validationRuleID');
    if (seen.has(id)) return t('settings.pricingFallback.validationDuplicateID');
    seen.add(id);

    if (rule.meter === 'image') {
      const credits = Number(rule.credits ?? 0);
      if (!Number.isFinite(credits) || credits <= 0) {
        return t('settings.pricingFallback.validationImageCredits');
      }
      continue;
    }

    if (!isValidNonNegativeNumber(rule.price_usd_per_1m_tokens)) {
      return t('settings.pricingFallback.validationTokenPrice');
    }
  }

  return '';
}

function sourceVariant(source: PricingFallbackSource | undefined) {
  return source === 'admin_fallback' ? 'default' : 'outline';
}

function sourceLabel(source: PricingFallbackSource | undefined, t: (key: SettingsKey) => string) {
  if (!source) return t('settings.pricingFallback.sourceUnknown');
  return t(SOURCE_LABEL_KEYS[source]);
}

function groupTokenRules(rules: PricingFallbackRule[]): TokenRuleGroup[] {
  const map = new Map<string, TokenRuleGroup>();

  for (const rule of rules) {
    if (rule.meter !== 'input_token' && rule.meter !== 'output_token') continue;

    const provider = rule.provider || '*';
    const model = rule.model || '*';
    const key = `${rule.operation}:${provider}:${model}`;
    const group =
      map.get(key) ??
      ({
        key,
        operation: rule.operation,
        provider,
        model,
      } satisfies TokenRuleGroup);

    if (rule.meter === 'input_token') group.input = rule;
    if (rule.meter === 'output_token') group.output = rule;
    map.set(key, group);
  }

  return Array.from(map.values()).sort((a, b) => {
    const opA = TOKEN_OPERATIONS.indexOf(a.operation);
    const opB = TOKEN_OPERATIONS.indexOf(b.operation);
    if (opA !== opB) return opA - opB;
    return a.key.localeCompare(b.key);
  });
}

function imageRules(rules: PricingFallbackRule[]) {
  return rules
    .filter(rule => rule.meter === 'image')
    .sort((a, b) => Number(matchText(a.provider) === '*') - Number(matchText(b.provider) === '*'));
}

function pricingSourceCount(rules: PricingFallbackRule[], source: PricingFallbackSource) {
  return rules.filter(rule => rule.pricing_source === source).length;
}

function formatTokenPrice(rule: PricingFallbackRule | undefined, t: (key: SettingsKey) => string) {
  return rule?.price_usd_per_1m_tokens
    ? `${rule.price_usd_per_1m_tokens} ${t('settings.pricingFallback.usdPerMillionShort')}`
    : '-';
}

function formatImageConditions(rule: PricingFallbackRule, t: (key: SettingsKey) => string) {
  return [
    `${t('settings.pricingFallback.size')}: ${displayPattern(rule.size, t('settings.pricingFallback.all'))}`,
    `${t('settings.pricingFallback.quality')}: ${displayPattern(rule.quality, t('settings.pricingFallback.all'))}`,
    `${t('settings.pricingFallback.style')}: ${displayPattern(rule.style, t('settings.pricingFallback.all'))}`,
  ].join(' / ');
}

function displayImageProvider(rule: PricingFallbackRule, t: (key: SettingsKey) => string) {
  return matchText(rule.provider) === '*'
    ? t('settings.pricingFallback.otherProviders')
    : displayPattern(rule.provider, t('settings.pricingFallback.allProviders'));
}

export function PricingFallbackPanel() {
  const t = useT();
  const [pricingFallback, setPricingFallback] = useState<PricingFallbackConfig | null>(null);
  const [overrideRules, setOverrideRules] = useState<PricingFallbackRule[]>([]);
  const [fallbackEnabled, setFallbackEnabled] = useState(true);
  const [fallbackLoading, setFallbackLoading] = useState(true);
  const [fallbackSaving, setFallbackSaving] = useState(false);
  const [overrideRulesError, setOverrideRulesError] = useState('');
  const [showDefaultRules, setShowDefaultRules] = useState(false);
  const fallbackLoadFailedText = t('settings.pricingFallback.loadFailed');

  const loadPricingFallback = useCallback(async () => {
    setFallbackLoading(true);
    try {
      const resp = await modelService.getPricingFallback();
      setPricingFallback(resp.data);
      setFallbackEnabled(resp.data.enabled);
      setOverrideRules(resp.data.override_rules ?? []);
      setOverrideRulesError('');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : fallbackLoadFailedText);
    } finally {
      setFallbackLoading(false);
    }
  }, [fallbackLoadFailedText]);

  useEffect(() => {
    void loadPricingFallback();
  }, [loadPricingFallback]);

  const findOverrideRule = useCallback(
    (baseRule: PricingFallbackRule) => overrideRules.find(rule => sameRuleTarget(rule, baseRule)),
    [overrideRules]
  );

  const getEditableRule = useCallback(
    (baseRule: PricingFallbackRule) => mergeEditableRule(baseRule, findOverrideRule(baseRule)),
    [findOverrideRule]
  );

  const hasOverrideRule = useCallback(
    (baseRule: PricingFallbackRule) => Boolean(findOverrideRule(baseRule)),
    [findOverrideRule]
  );

  const upsertOverrideRule = useCallback(
    (baseRule: PricingFallbackRule, patch: Partial<PricingFallbackRule>) => {
      setOverrideRules(rules => {
        const index = rules.findIndex(rule => sameRuleTarget(rule, baseRule));
        const existingRule = index >= 0 ? rules[index] : undefined;
        const nextRule = buildOverrideRule(baseRule, patch, existingRule);

        if (index < 0) {
          return [...rules, nextRule];
        }

        return rules.map((rule, ruleIndex) => (ruleIndex === index ? nextRule : rule));
      });
    },
    []
  );

  const resetOverrideRule = useCallback((baseRule: PricingFallbackRule) => {
    setOverrideRules(rules => rules.filter(rule => !sameRuleTarget(rule, baseRule)));
  }, []);

  const savePricingFallback = async () => {
    setOverrideRulesError('');
    const normalizedRules = overrideRules.map(normalizeRuleForSave);
    const validationError = validateRules(normalizedRules, t);
    if (validationError) {
      setOverrideRulesError(validationError);
      return;
    }

    setFallbackSaving(true);
    try {
      const resp = await modelService.updatePricingFallback({
        enabled: fallbackEnabled,
        override_rules: normalizedRules,
      });
      setPricingFallback(resp.data);
      setFallbackEnabled(resp.data.enabled);
      setOverrideRules(resp.data.override_rules ?? []);
      toast.success(t('settings.pricingFallback.saved'));
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('settings.pricingFallback.saveFailed')
      );
    } finally {
      setFallbackSaving(false);
    }
  };

  const isBusy = fallbackLoading || fallbackSaving;
  const defaultRules = pricingFallback?.default_rules ?? [];
  const effectiveRules = pricingFallback?.effective_rules ?? [];
  const adminRuleCount = overrideRules.length;
  const codeRuleCount = pricingSourceCount(effectiveRules, 'code_default_fallback');

  return (
    <div className="rounded-md border border-border bg-background p-5 shadow-sm">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-sm font-medium">{t('settings.pricingFallback.title')}</h3>
            <Badge
              variant={pricingFallback?.enabled ? 'success' : 'outline'}
              className="rounded-full"
            >
              {pricingFallback?.enabled ? t('settings.system.active') : t('settings.system.later')}
            </Badge>
            <Badge variant="outline" className="rounded-full">
              {adminRuleCount} {t('settings.pricingFallback.adminRuleCount')}
            </Badge>
            <Badge variant="outline" className="rounded-full">
              {codeRuleCount} {t('settings.pricingFallback.codeRuleCount')}
            </Badge>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('settings.pricingFallback.description')}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            className="gap-2"
            onClick={() => void loadPricingFallback()}
            disabled={isBusy}
          >
            <RefreshCw className={`h-4 w-4 ${fallbackLoading ? 'animate-spin' : ''}`} />
            {t('settings.pricingFallback.refresh')}
          </Button>
          <Button
            size="sm"
            className="gap-2"
            onClick={() => void savePricingFallback()}
            disabled={isBusy}
          >
            {fallbackSaving ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Save className="h-4 w-4" />
            )}
            {t('settings.pricingFallback.save')}
          </Button>
        </div>
      </div>

      <PolicyStrip
        enabled={fallbackEnabled}
        disabled={isBusy}
        onEnabledChange={setFallbackEnabled}
      />

      {overrideRulesError ? (
        <p className="mt-4 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">
          {overrideRulesError}
        </p>
      ) : null}

      <section className="mt-4 rounded-md border border-border bg-background p-4">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <h4 className="text-sm font-medium">
              {t('settings.pricingFallback.effectiveSummaryTitle')}
            </h4>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">
              {t('settings.pricingFallback.effectiveSummaryDesc')}
            </p>
          </div>
        </div>
        {fallbackLoading ? (
          <LoadingBlock />
        ) : (
          <RuleSummaryBoard
            rules={effectiveRules}
            emptyText={t('settings.pricingFallback.emptyEffectiveRules')}
            editable
            disabled={isBusy}
            getEditableRule={getEditableRule}
            hasOverrideRule={hasOverrideRule}
            onRuleChange={upsertOverrideRule}
            onRuleReset={resetOverrideRule}
          />
        )}
      </section>

      <section className="mt-4 rounded-md border border-border bg-background">
        <button
          type="button"
          className="flex w-full items-center justify-between gap-3 px-4 py-3 text-left"
          onClick={() => setShowDefaultRules(v => !v)}
        >
          <div>
            <h4 className="text-sm font-medium">{t('settings.pricingFallback.defaultRules')}</h4>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">
              {t('settings.pricingFallback.defaultRulesDesc')}
            </p>
          </div>
          {showDefaultRules ? (
            <ChevronUp className="h-4 w-4 text-muted-foreground" />
          ) : (
            <ChevronDown className="h-4 w-4 text-muted-foreground" />
          )}
        </button>
        {showDefaultRules ? (
          <div className="border-t border-border p-4">
            {fallbackLoading ? (
              <LoadingBlock />
            ) : (
              <RuleSummaryBoard
                rules={defaultRules}
                emptyText={t('settings.pricingFallback.emptyDefaultRules')}
              />
            )}
          </div>
        ) : null}
      </section>
    </div>
  );
}

function PolicyStrip({
  enabled,
  disabled,
  onEnabledChange,
}: {
  enabled: boolean;
  disabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
}) {
  const t = useT();

  return (
    <div className="mt-4 rounded-md border border-border bg-muted/20 p-3">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div className="grid flex-1 gap-3 md:grid-cols-3">
          <PolicyStep index="1" title={t('settings.pricingFallback.policyUpstream')} />
          <PolicyStep index="2" title={t('settings.pricingFallback.policyFallback')} />
          <PolicyStep index="3" title={t('settings.pricingFallback.policyFailFirst')} warning />
        </div>
        <div className="flex items-center justify-between gap-3 rounded-md border border-border bg-muted/20 px-3 py-2 lg:min-w-[260px]">
          <div>
            <Label className="text-sm">{t('settings.pricingFallback.enableFallback')}</Label>
            <p className="mt-1 text-[11px] text-muted-foreground">
              {t('settings.pricingFallback.enableFallbackShortDesc')}
            </p>
          </div>
          <Switch checked={enabled} onCheckedChange={onEnabledChange} disabled={disabled} />
        </div>
      </div>
      <p className="mt-3 text-xs leading-5 text-muted-foreground">
        {t('settings.pricingFallback.missingRuleDesc')}
      </p>
    </div>
  );
}

function PolicyStep({
  index,
  title,
  warning,
}: {
  index: string;
  title: string;
  warning?: boolean;
}) {
  return (
    <div className="flex items-center gap-2 rounded-md bg-muted/30 px-3 py-2">
      <span
        className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-medium ${
          warning ? 'bg-amber-100 text-amber-800' : 'bg-primary/10 text-primary'
        }`}
      >
        {index}
      </span>
      <span className="text-sm font-medium">{title}</span>
    </div>
  );
}

function LoadingBlock() {
  const t = useT();

  return (
    <div className="mt-3 rounded-md border border-border bg-muted/20 p-5 text-sm text-muted-foreground">
      {t('settings.pricingFallback.loading')}
    </div>
  );
}

function RuleSummaryBoard({
  rules,
  emptyText,
  editable = false,
  disabled = false,
  getEditableRule,
  hasOverrideRule,
  onRuleChange,
  onRuleReset,
}: {
  rules: PricingFallbackRule[];
  emptyText: string;
  editable?: boolean;
  disabled?: boolean;
  getEditableRule?: (rule: PricingFallbackRule) => PricingFallbackRule;
  hasOverrideRule?: (rule: PricingFallbackRule) => boolean;
  onRuleChange?: (rule: PricingFallbackRule, patch: Partial<PricingFallbackRule>) => void;
  onRuleReset?: (rule: PricingFallbackRule) => void;
}) {
  const t = useT();
  const tokenRuleGroups = useMemo(() => groupTokenRules(rules), [rules]);
  const imageRuleList = useMemo(() => imageRules(rules), [rules]);
  const ruleForDisplay = useCallback(
    (rule: PricingFallbackRule) => (getEditableRule ? getEditableRule(rule) : rule),
    [getEditableRule]
  );
  const isOverridden = useCallback(
    (rule: PricingFallbackRule | undefined) => Boolean(rule && hasOverrideRule?.(rule)),
    [hasOverrideRule]
  );

  if (rules.length === 0) {
    return (
      <div className="mt-3 rounded-md border border-dashed border-border bg-muted/20 p-5 text-sm text-muted-foreground">
        {emptyText}
      </div>
    );
  }

  return (
    <div className="mt-3 space-y-5">
      <div>
        <div className="mb-2 flex items-center gap-2">
          <Type className="h-4 w-4 text-muted-foreground" />
          <h5 className="text-sm font-medium">
            {t('settings.pricingFallback.tokenFallbackTitle')}
          </h5>
        </div>
        {tokenRuleGroups.length === 0 ? (
          <p className="rounded-md border border-dashed border-border bg-muted/20 p-4 text-sm text-muted-foreground">
            {t('settings.pricingFallback.noTokenRules')}
          </p>
        ) : (
          <div className="overflow-hidden rounded-md border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('settings.pricingFallback.operation')}</TableHead>
                  <TableHead>{t('settings.pricingFallback.matchScope')}</TableHead>
                  <TableHead>{t('settings.pricingFallback.meterInput')}</TableHead>
                  <TableHead>{t('settings.pricingFallback.meterOutput')}</TableHead>
                  <TableHead>{t('settings.pricingFallback.columnSource')}</TableHead>
                  {editable ? (
                    <TableHead className="text-right">
                      {t('settings.pricingFallback.columnAction')}
                    </TableHead>
                  ) : null}
                </TableRow>
              </TableHeader>
              <TableBody>
                {tokenRuleGroups.map(group => {
                  const inputRule = group.input ? ruleForDisplay(group.input) : undefined;
                  const outputRule = group.output ? ruleForDisplay(group.output) : undefined;
                  const hasOverride = isOverridden(group.input) || isOverridden(group.output);

                  return (
                    <TableRow key={group.key}>
                      <TableCell className="font-medium">
                        {t(OPERATION_LABEL_KEYS[group.operation])}
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {displayPattern(group.provider, t('settings.pricingFallback.allProviders'))}{' '}
                        / {displayPattern(group.model, t('settings.pricingFallback.allModels'))}
                      </TableCell>
                      <TableCell className="min-w-[210px]">
                        <TokenPriceCell
                          baseRule={group.input}
                          displayRule={inputRule}
                          editable={editable}
                          disabled={disabled}
                          onRuleChange={onRuleChange}
                        />
                      </TableCell>
                      <TableCell className="min-w-[210px]">
                        <TokenPriceCell
                          baseRule={group.output}
                          displayRule={outputRule}
                          editable={editable}
                          disabled={disabled}
                          onRuleChange={onRuleChange}
                        />
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant={sourceVariant(
                            hasOverride
                              ? 'admin_fallback'
                              : (group.input?.pricing_source ?? group.output?.pricing_source)
                          )}
                          className="rounded-full"
                        >
                          {sourceLabel(
                            hasOverride
                              ? 'admin_fallback'
                              : (group.input?.pricing_source ?? group.output?.pricing_source),
                            t
                          )}
                        </Badge>
                      </TableCell>
                      {editable ? (
                        <TableCell className="text-right">
                          <ResetRulesButton
                            rules={[group.input, group.output]}
                            visible={hasOverride}
                            disabled={disabled}
                            onRuleReset={onRuleReset}
                          />
                        </TableCell>
                      ) : null}
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </div>
        )}
      </div>

      <div>
        <div className="mb-2 flex items-center gap-2">
          <ImageIcon className="h-4 w-4 text-muted-foreground" />
          <h5 className="text-sm font-medium">
            {t('settings.pricingFallback.imageFallbackTitle')}
          </h5>
        </div>
        {imageRuleList.length === 0 ? (
          <p className="rounded-md border border-dashed border-border bg-muted/20 p-4 text-sm text-muted-foreground">
            {t('settings.pricingFallback.noImageRules')}
          </p>
        ) : (
          <div className="overflow-hidden rounded-md border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('settings.pricingFallback.matchScope')}</TableHead>
                  <TableHead>{t('settings.pricingFallback.imageConditions')}</TableHead>
                  <TableHead>{t('settings.pricingFallback.columnPrice')}</TableHead>
                  <TableHead>{t('settings.pricingFallback.columnSource')}</TableHead>
                  {editable ? (
                    <TableHead className="text-right">
                      {t('settings.pricingFallback.columnAction')}
                    </TableHead>
                  ) : null}
                </TableRow>
              </TableHeader>
              <TableBody>
                {imageRuleList.map(rule => {
                  const displayRule = ruleForDisplay(rule);
                  const hasOverride = isOverridden(rule);

                  return (
                    <TableRow key={rule.id}>
                      <TableCell className="font-medium">
                        {displayImageProvider(rule, t)} /{' '}
                        {displayPattern(rule.model, t('settings.pricingFallback.allModels'))}
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {formatImageConditions(rule, t)}
                      </TableCell>
                      <TableCell className="min-w-[210px]">
                        <ImagePriceCell
                          baseRule={rule}
                          displayRule={displayRule}
                          editable={editable}
                          disabled={disabled}
                          onRuleChange={onRuleChange}
                        />
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant={sourceVariant(
                            hasOverride ? 'admin_fallback' : rule.pricing_source
                          )}
                          className="rounded-full"
                        >
                          {sourceLabel(hasOverride ? 'admin_fallback' : rule.pricing_source, t)}
                        </Badge>
                      </TableCell>
                      {editable ? (
                        <TableCell className="text-right">
                          <ResetRulesButton
                            rules={[rule]}
                            visible={hasOverride}
                            disabled={disabled}
                            onRuleReset={onRuleReset}
                          />
                        </TableCell>
                      ) : null}
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </div>
        )}
      </div>
    </div>
  );
}

function TokenPriceCell({
  baseRule,
  displayRule,
  editable,
  disabled,
  onRuleChange,
}: {
  baseRule?: PricingFallbackRule;
  displayRule?: PricingFallbackRule;
  editable: boolean;
  disabled: boolean;
  onRuleChange?: (rule: PricingFallbackRule, patch: Partial<PricingFallbackRule>) => void;
}) {
  const t = useT();

  if (!baseRule || !displayRule) return <span className="text-muted-foreground">-</span>;

  if (!editable) return <>{formatTokenPrice(displayRule, t)}</>;

  return (
    <div className="flex items-center gap-2">
      <Input
        type="number"
        min="0"
        step="0.0001"
        className="h-8 w-28"
        value={displayRule.price_usd_per_1m_tokens ?? ''}
        onChange={event =>
          onRuleChange?.(baseRule, { price_usd_per_1m_tokens: event.target.value })
        }
        disabled={disabled}
      />
      <span className="text-xs text-muted-foreground">
        {t('settings.pricingFallback.usdPerMillionShort')}
      </span>
    </div>
  );
}

function ImagePriceCell({
  baseRule,
  displayRule,
  editable,
  disabled,
  onRuleChange,
}: {
  baseRule: PricingFallbackRule;
  displayRule: PricingFallbackRule;
  editable: boolean;
  disabled: boolean;
  onRuleChange?: (rule: PricingFallbackRule, patch: Partial<PricingFallbackRule>) => void;
}) {
  const t = useT();

  if (!editable) {
    return (
      <>
        {displayRule.credits ?? '-'} {t('settings.pricingFallback.creditsPerImage')}
      </>
    );
  }

  return (
    <div className="flex items-center gap-2">
      <Input
        type="number"
        min="1"
        step="1"
        className="h-8 w-28"
        value={displayRule.credits ?? ''}
        onChange={event =>
          onRuleChange?.(baseRule, {
            credits: event.target.value === '' ? undefined : Number(event.target.value),
          })
        }
        disabled={disabled}
      />
      <span className="text-xs text-muted-foreground">
        {t('settings.pricingFallback.creditsPerImage')}
      </span>
    </div>
  );
}

function ResetRulesButton({
  rules,
  visible,
  disabled,
  onRuleReset,
}: {
  rules: Array<PricingFallbackRule | undefined>;
  visible: boolean;
  disabled: boolean;
  onRuleReset?: (rule: PricingFallbackRule) => void;
}) {
  const t = useT();

  if (!visible) return <span className="text-muted-foreground">-</span>;

  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="gap-2"
      disabled={disabled}
      onClick={() => rules.forEach(rule => rule && onRuleReset?.(rule))}
    >
      <RotateCcw className="h-4 w-4" />
      {t('settings.pricingFallback.resetDefault')}
    </Button>
  );
}
