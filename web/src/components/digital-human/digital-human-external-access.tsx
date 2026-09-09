'use client';

import { useEffect, useRef, useState } from 'react';
import { AppWindow } from 'lucide-react';
import { toast } from 'sonner';
import { DigitalHumanAccessPanel } from '@/components/digital-human/digital-human-access-panel';
import {
  useDigitalHumanDraftStore,
  useRegisterDigitalHumanDraftEditor,
} from '@/components/digital-human/digital-human-draft-store';
import { useDigitalHumanDraftPersistence } from '@/components/digital-human/use-digital-human-draft-persistence';
import {
  useDigitalHuman,
  useDigitalHumanMutations,
} from '@/hooks/digital-human/use-digital-humans';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import type { DigitalHumanPresentationPolicy } from '@/services/types/digital-human';

export function DigitalHumanExternalAccess({ id }: { id: string }) {
  const detailQuery = useDigitalHuman(id);
  const detail = detailQuery.data?.data;
  const resetVersion = useDigitalHumanDraftStore(state => state.resetVersions[id] ?? 0);

  if (detailQuery.isLoading || !detail) {
    return <DigitalHumanExternalAccessLoading />;
  }

  return <ExternalAccessEditor id={id} key={`${id}:${resetVersion}`} />;
}

function ExternalAccessEditor({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const permissions = useAccountPermissions();
  const detail = useDigitalHuman(id).data?.data;
  const mutations = useDigitalHumanMutations(id);
  const versionPreview = useDigitalHumanDraftStore(state => state.versionPreviews[id]);
  const canUpdate = permissions.hasPermission('digital_human.update');
  const editingEnabled = canUpdate && !versionPreview;
  const initialPresentation = normalizePresentationPolicy(detail?.draft.presentation_policy);
  const [presentation, setPresentation] = useState(initialPresentation);
  const draftLockVersionRef = useRef(detail?.draft.lock_version ?? 0);

  useEffect(() => {
    if (detail) draftLockVersionRef.current = detail.draft.lock_version;
  }, [detail]);

  const normalizedPresentation: DigitalHumanPresentationPolicy = {
    ...presentation,
    suggested_questions: presentation.suggested_questions
      .map(item => item.trim())
      .filter(Boolean)
      .slice(0, 6),
  };
  const presentationValid =
    !normalizedPresentation.theme_color.trim() ||
    /^#[0-9a-f]{6}$/i.test(normalizedPresentation.theme_color.trim());
  const persistence = useDigitalHumanDraftPersistence({
    currentPayload: normalizedPresentation,
    initialPayload: initialPresentation,
    enabled: editingEnabled && Boolean(detail),
    canSave: () => presentationValid,
    savePayload: async value => {
      const result = await mutations.updateDraft.mutateAsync({
        digitalHumanId: id,
        request: {
          expected_lock_version: draftLockVersionRef.current,
          presentation_policy: value,
        },
      });
      draftLockVersionRef.current = result.data.lock_version;
      return { savedPayload: value, updatedAt: Date.now() };
    },
  });

  const saveNow = async () => {
    if (versionPreview) return false;
    if (!presentationValid) {
      toast.error(t('access.invalidThemeColor'));
      return false;
    }
    return persistence.saveNow({ force: true });
  };

  useRegisterDigitalHumanDraftEditor(id, {
    canUpdate: editingEnabled,
    saveState: persistence.saveState,
    lastSavedAt: persistence.lastSavedAt,
    isDirty: persistence.isDirty,
    isSaving: persistence.isSaving,
    saveNow,
  });

  if (!detail) return null;
  const displayedPresentation = versionPreview
    ? normalizePresentationPolicy(versionPreview.version.presentation_policy)
    : presentation;

  return (
    <div className="mx-auto min-h-full w-full max-w-[1200px] px-4 py-6 md:px-8 xl:py-8">
      {versionPreview ? (
        <div className="mb-5 rounded-lg border border-primary/20 bg-primary/[0.04] px-4 py-3 text-sm text-primary">
          {t('versionHistory.previewBanner', {
            version: versionPreview.version.version_number ?? '',
          })}
        </div>
      ) : null}
      <header className="flex items-start gap-3 border-b pb-5">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
          <AppWindow className="size-5" />
        </span>
        <div className="min-w-0">
          <h2 className="text-xl font-semibold">{t('access.landingPageTitle')}</h2>
          <p className="mt-1 max-w-3xl text-sm leading-6 text-muted-foreground">
            {t('access.landingPageDescription')}
          </p>
        </div>
      </header>
      <DigitalHumanAccessPanel
        id={id}
        definition={detail.definition}
        published={Boolean(detail.published)}
        canPublish={permissions.hasPermission('digital_human.publish') && !versionPreview}
        canUpdatePresentation={editingEnabled}
        presentation={displayedPresentation}
        onPresentationChange={setPresentation}
      />
    </div>
  );
}

function DigitalHumanExternalAccessLoading() {
  return (
    <div className="mx-auto w-full max-w-[1200px] space-y-5 px-4 py-6 md:px-8 xl:py-8">
      <div className="h-12 w-72 animate-pulse rounded-lg bg-muted" />
      <div className="h-[560px] animate-pulse rounded-xl bg-muted/50" />
    </div>
  );
}

function normalizePresentationPolicy(
  policy?: Partial<DigitalHumanPresentationPolicy>
): DigitalHumanPresentationPolicy {
  return {
    home_title: policy?.home_title ?? '',
    opening_statement: policy?.opening_statement ?? '',
    input_placeholder: policy?.input_placeholder ?? '',
    theme_color: policy?.theme_color ?? '',
    suggested_questions: (policy?.suggested_questions ?? []).slice(0, 6),
  };
}
