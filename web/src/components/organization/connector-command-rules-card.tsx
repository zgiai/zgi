'use client';

import { useState, type FormEvent } from 'react';
import { Terminal, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useConnectorCommandRules } from '@/hooks/organization/use-connector-command-rules';
import { useT } from '@/i18n';
import type { ConnectorCommandRuleAction } from '@/services/types/connector';
import { getErrorMessage } from '@/utils/error-notifications';

/**
 * Organization rules for the server commands DigitalHumans run (ADR 0029).
 * They only tighten: a forbidden command never runs, and an always-ask command
 * needs a person's approval even on a trusted connection.
 */
export function ConnectorCommandRulesCard() {
  const t = useT('dashboard');
  const { query, create, remove } = useConnectorCommandRules(true);
  const [prefix, setPrefix] = useState('');
  const [action, setAction] = useState<ConnectorCommandRuleAction>('forbid');
  const [note, setNote] = useState('');
  const rules = query.data?.data.items ?? [];
  const canManage = query.data?.data.can_manage === true;

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!prefix.trim()) return;
    try {
      await create.mutateAsync({ prefix: prefix.trim(), action, note: note.trim() });
      setPrefix('');
      setNote('');
    } catch (error) {
      toast.error(getErrorMessage(error) || t('organization.settings.commandRules.createFailed'));
    }
  };

  return (
    <section className="overflow-hidden rounded-xl border border-border/80 bg-background shadow-sm">
      <div className="flex gap-3 border-b border-border/60 bg-muted/20 px-5 py-4">
        <div className="flex size-8 shrink-0 items-center justify-center rounded-lg border border-primary/20 bg-primary/10 text-primary">
          <Terminal className="size-4" />
        </div>
        <div className="min-w-0">
          <h2 className="text-sm font-semibold text-text-primary">
            {t('organization.settings.commandRules.title')}
          </h2>
          <p className="mt-1 text-xs leading-5 text-text-secondary">
            {t('organization.settings.commandRules.description')}
          </p>
        </div>
      </div>
      <div className="space-y-4 px-5 py-4">
        {canManage ? (
          <form
            className="grid gap-3 md:grid-cols-[2fr_1fr_2fr_auto] md:items-end"
            onSubmit={submit}
          >
            <div className="space-y-1.5">
              <Label htmlFor="command-rule-prefix">
                {t('organization.settings.commandRules.prefix')}
              </Label>
              <Input
                id="command-rule-prefix"
                value={prefix}
                maxLength={200}
                placeholder={t('organization.settings.commandRules.prefixPlaceholder')}
                onChange={event => setPrefix(event.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label>{t('organization.settings.commandRules.action')}</Label>
              <Select
                value={action}
                onValueChange={value => setAction(value as ConnectorCommandRuleAction)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="forbid">
                    {t('organization.settings.commandRules.forbid')}
                  </SelectItem>
                  <SelectItem value="ask">{t('organization.settings.commandRules.ask')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="command-rule-note">
                {t('organization.settings.commandRules.note')}
              </Label>
              <Input
                id="command-rule-note"
                value={note}
                maxLength={200}
                onChange={event => setNote(event.target.value)}
              />
            </div>
            <Button type="submit" disabled={!prefix.trim() || create.isPending}>
              {t('organization.settings.commandRules.add')}
            </Button>
          </form>
        ) : null}
        <p className="text-xs leading-5 text-text-secondary">
          {t('organization.settings.commandRules.matching')}
        </p>
        {query.isError ? (
          <p role="alert" className="text-sm text-destructive">
            {t('organization.settings.commandRules.loadFailed')}
          </p>
        ) : rules.length === 0 ? (
          <p className="text-sm text-text-secondary">
            {t('organization.settings.commandRules.empty')}
          </p>
        ) : (
          <ul className="divide-y divide-border/60 rounded-lg border border-border/60">
            {rules.map(rule => (
              <li key={rule.id} className="flex items-center gap-3 px-3 py-2">
                <Badge variant={rule.action === 'forbid' ? 'destructive' : 'outline'}>
                  {t(
                    rule.action === 'forbid'
                      ? 'organization.settings.commandRules.forbid'
                      : 'organization.settings.commandRules.ask'
                  )}
                </Badge>
                <code className="min-w-0 flex-1 break-all text-sm">{rule.prefix}</code>
                {rule.note ? (
                  <span className="hidden max-w-[40%] truncate text-xs text-text-secondary md:block">
                    {rule.note}
                  </span>
                ) : null}
                {canManage ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    aria-label={t('organization.settings.commandRules.remove', {
                      prefix: rule.prefix,
                    })}
                    disabled={remove.isPending}
                    onClick={() =>
                      remove
                        .mutateAsync(rule.id)
                        .catch(error => toast.error(getErrorMessage(error)))
                    }
                  >
                    <Trash2 className="size-4" />
                  </Button>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
