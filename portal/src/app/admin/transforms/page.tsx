/* Hallmark · genre: modern-minimal · macrostructure: Workbench
 * design-system: design.md · designed-as-app
 */
'use client';

import React, { useState, useEffect } from 'react';
import { useSearchParams } from 'next/navigation';
import {
  useQuery,
  useMutation,
  useQueryClient,
  type UseMutationResult,
} from '@tanstack/react-query';
import { api } from '@/lib/api';
import NavigationShell from '@/components/NavigationShell';
import {
  Button,
  Field,
  PageHeader,
  SectionCard,
  EmptyState,
  Notice,
  SkeletonLine,
  Loading,
  useToast,
  useConfirm,
  SearchableSelect,
} from '@/components/ui';
import { Plus, Trash2, Pencil, Sliders } from 'lucide-react';

interface TransformMatch {
  path: string;
  methods?: string[];
  options?: Record<string, unknown>;
}

interface TransformConfig {
  id?: string;
  name: string;
  description?: string;
  serviceId: string;
  match: TransformMatch;
  enabled: boolean;
  rules: unknown[];
}

interface Preset {
  name: string;
  types: string[];
  title: string;
  description: string;
}

interface ServiceOption {
  id: string;
  name: string;
  basePath: string;
  type?: string;
}

function messageOf(err: unknown, fallback: string) {
  return err instanceof Error && err.message ? err.message : fallback;
}

export default function AdminTransformsPage() {
  const queryClient = useQueryClient();
  const { toast, error: toastError } = useToast();
  const confirm = useConfirm();

  const searchParams = useSearchParams();
  const urlServiceId = searchParams.get('serviceId');

  const [selectedServiceId, setSelectedServiceId] = useState(urlServiceId || '');
  const [isEditorOpen, setIsEditorOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);

  const [transformName, setTransformName] = useState('');
  const [transformDescription, setTransformDescription] = useState('');
  const [matchPath, setMatchPath] = useState('/*');
  const [enabled, setEnabled] = useState(true);
  const [rulesJson, setRulesJson] = useState('[]');
  const [showPresetsCard, setShowPresetsCard] = useState(false);

  const { data: services } = useQuery<ServiceOption[]>({
    queryKey: ['admin-transform-services'],
    queryFn: async () => {
      const res = await api.get('/services');
      return res.data.items || [];
    },
  });

  // ปรับ state ระหว่าง render ตามแนวทางของ React ไม่ใช่ใน effect
  if (!selectedServiceId && services && services.length > 0) {
    const target = urlServiceId && services.some((s) => s.id === urlServiceId)
      ? urlServiceId
      : services[0].id;
    setSelectedServiceId(target);
  }

  const selectedService = services?.find((s) => s.id === selectedServiceId);

  const { data: transforms, isLoading: isLoadingTransforms } = useQuery<TransformConfig[]>({
    queryKey: ['transforms-for-service', selectedServiceId],
    queryFn: async () => {
      const res = await api.get(`/response-transforms?serviceId=${selectedServiceId}`);
      return res.data.items || [];
    },
    enabled: !!selectedServiceId,
  });

  const { data: presets } = useQuery<Preset[]>({
    queryKey: ['presets-for-type', selectedService?.type],
    queryFn: async () => {
      const res = await api.get(`/response-transforms/presets?type=${selectedService?.type}`);
      return res.data.items || [];
    },
    enabled: !!selectedService?.type,
  });

  // ตรวจ JSON ทันทีที่พิมพ์ ไม่ต้องรอกดบันทึกถึงจะรู้ว่าพัง
  let jsonError: string | null = null;
  try {
    const parsed = JSON.parse(rulesJson);
    if (!Array.isArray(parsed)) jsonError = 'This has to be an array of rules, not an object.';
  } catch (err) {
    jsonError = messageOf(err, 'That is not valid JSON.');
  }

  /** เปิด editor พร้อมเติมค่า — ทำตรงนี้ทีเดียว ไม่ต้องมี effect คอยตามซิงก์ */
  function openEditor(t: TransformConfig | null) {
    setEditingId(t?.id ?? null);
    setTransformName(t?.name ?? '');
    setTransformDescription(t?.description ?? '');
    setMatchPath(t?.match?.path ?? '/*');
    setEnabled(t?.enabled !== false);
    setRulesJson(JSON.stringify(t?.rules ?? [], null, 2));
    setIsEditorOpen(true);
  }

  function closeEditor() {
    setIsEditorOpen(false);
    setEditingId(null);
  }

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ['transforms-for-service', selectedServiceId] });

  const applyPreset: UseMutationResult<TransformConfig, Error, string> = useMutation({
    mutationFn: async (presetName: string) => {
      const res = await api.post(`/response-transforms/presets/${presetName}/apply`, {
        serviceId: selectedServiceId,
      });
      return res.data;
    },
    onSuccess: (created) => {
      invalidate();
      if (created) {
        openEditor(created);
      }
      toast({ tone: 'ok', message: 'Preset applied (disabled) - review rules and enable when ready' });
    },
    onError: (err, name) =>
      toastError(messageOf(err, 'Could not apply the preset'), () => applyPreset.mutate(name)),
  });

  const saveTransform: UseMutationResult<void, Error, void> = useMutation({
    mutationFn: async () => {
      const payload = {
        name: transformName,
        description: transformDescription,
        serviceId: selectedServiceId,
        match: { path: matchPath, methods: ['GET'] },
        enabled,
        rules: JSON.parse(rulesJson) as unknown[],
      };
      if (editingId) await api.put(`/response-transforms/${editingId}`, payload);
      else await api.post('/response-transforms', payload);
    },
    onSuccess: () => {
      invalidate();
      toast({ tone: 'ok', message: 'Ruleset saved' });
      closeEditor();
    },
    onError: (err) => toastError(messageOf(err, 'Could not save the ruleset')),
  });

  const deleteTransform: UseMutationResult<void, Error, string> = useMutation({
    mutationFn: async (id: string) => {
      await api.delete(`/response-transforms/${id}`);
    },
    onSuccess: () => {
      invalidate();
      toast({ tone: 'ok', message: 'Ruleset deleted' });
      if (editingId) closeEditor();
    },
    onError: (err, id) =>
      toastError(messageOf(err, 'Could not delete the ruleset'), () => deleteTransform.mutate(id)),
  });

  async function askDelete(t: TransformConfig, e?: React.MouseEvent) {
    if (e) e.stopPropagation();
    const ok = await confirm({
      title: `Delete ruleset "${t.name}"`,
      consequences: [
        'This cannot be undone',
        'Links in this service responses go back to pointing at the upstream server',
      ],
      confirmLabel: 'Delete ruleset',
      danger: true,
    });
    if (ok && t.id) deleteTransform.mutate(t.id);
  }

  const hasPresets = presets && presets.length > 0;
  const showPresetOffer = hasPresets && (transforms?.length === 0 || showPresetsCard);

  return (
    <NavigationShell>
      <PageHeader
        title="Transforms & Presets"
        description="Rewrite the links a service returns, so clients come back through the gateway instead of going straight upstream"
      />

      <SectionCard className="mb-6" title="Service">
        <div className="max-w-md">
          <SearchableSelect
            label="Which service these rules apply to"
            value={selectedServiceId}
            onChange={(val) => {
              setSelectedServiceId(val);
              closeEditor();
            }}
            options={
              services?.map((svc) => ({
                value: svc.id,
                label: svc.name,
                subtext: svc.basePath,
              })) || []
            }
            placeholder="Select a service..."
            searchPlaceholder="Search services by name or path..."
          />
        </div>
      </SectionCard>

      {selectedServiceId && (
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
          <div className="space-y-6 lg:col-span-1">
            {showPresetOffer && (
              <SectionCard
                title="Available Templates"
                description="Ready-made link rewriting rulesets for this service type"
                actions={
                  transforms && transforms.length > 0 ? (
                    <Button size="sm" variant="ghost" onClick={() => setShowPresetsCard(false)}>
                      Hide
                    </Button>
                  ) : undefined
                }
              >
                <div className="space-y-2">
                  {presets.map((preset) => (
                    <button
                      key={preset.name}
                      type="button"
                      onClick={() => applyPreset.mutate(preset.name)}
                      disabled={applyPreset.isPending}
                      className="flex w-full items-center justify-between gap-2 rounded-control border border-rule bg-paper-2 p-3 text-left transition-colors duration-200 hover:border-accent-edge hover:bg-accent-wash disabled:opacity-55"
                    >
                      <span className="min-w-0">
                        <span className="block text-sm font-medium text-ink">{preset.title}</span>
                        <span className="mt-0.5 block text-xs text-muted">
                          {preset.description}
                        </span>
                      </span>
                      <Plus className="h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
                    </button>
                  ))}
                </div>
              </SectionCard>
            )}

            <SectionCard
              title="Rulesets"
              actions={
                <div className="flex items-center gap-1.5">
                  {hasPresets && transforms && transforms.length > 0 && !showPresetsCard && (
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={() => setShowPresetsCard(true)}
                    >
                      Templates
                    </Button>
                  )}
                  <Button
                    size="sm"
                    variant="primary"
                    icon={<Plus className="h-4 w-4" />}
                    onClick={() => openEditor(null)}
                  >
                    New
                  </Button>
                </div>
              }
              flush
            >
              {isLoadingTransforms && (
                <Loading label="Loading rulesets">
                  <div className="space-y-2 p-4">
                    {[0, 1].map((i) => (
                      <SkeletonLine key={i} className="h-8" />
                    ))}
                  </div>
                </Loading>
              )}

              {!isLoadingTransforms && transforms?.length === 0 && (
                <EmptyState
                  icon={<Sliders className="h-5 w-5" />}
                  title="No rulesets yet"
                  description="Apply a template above, or write your own from scratch."
                />
              )}

              {!isLoadingTransforms && transforms && transforms.length > 0 && (
                <ul className="divide-y divide-rule" role="list">
                  {transforms.map((t) => {
                    const isSelected = editingId === t.id && isEditorOpen;
                    return (
                      <li
                        key={t.id}
                        onClick={() => openEditor(t)}
                        className={`group flex cursor-pointer items-center justify-between gap-2 p-3.5 transition-colors duration-150 ${
                          isSelected
                            ? 'border-l-4 border-accent bg-accent-wash'
                            : 'hover:bg-paper-2'
                        }`}
                      >
                        <div className="min-w-0 flex-1">
                          <p className="flex items-center gap-2 truncate text-sm font-medium text-ink">
                            <span className="truncate">{t.name}</span>
                            {t.enabled === false && (
                              <span className="shrink-0 rounded-chip border border-rule bg-paper-3 px-1.5 py-0.5 text-2xs font-normal text-muted">
                                Disabled
                              </span>
                            )}
                          </p>
                          <p className="mt-0.5 truncate font-mono text-xs text-muted">{t.match?.path}</p>
                        </div>
                        <div className="flex shrink-0 items-center gap-1">
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={(e) => {
                              e.stopPropagation();
                              openEditor(t);
                            }}
                            aria-label={`Edit ruleset ${t.name}`}
                            icon={<Pencil className="h-4 w-4" />}
                          />
                          <Button
                            size="sm"
                            variant="ghost-danger"
                            onClick={(e) => askDelete(t, e)}
                            loading={deleteTransform.isPending && deleteTransform.variables === t.id}
                            aria-label={`Delete ruleset ${t.name}`}
                            icon={<Trash2 className="h-4 w-4" />}
                          />
                        </div>
                      </li>
                    );
                  })}
                </ul>
              )}
            </SectionCard>
          </div>

          <div className="lg:col-span-2">
            {!isEditorOpen && (
              <div className="ui-card">
                <EmptyState
                  icon={<Sliders className="h-5 w-5" />}
                  title="Pick a ruleset, or create one"
                  description="Select a ruleset on the left to review or edit its rules, or create a new one."
                  action={
                    <Button variant="secondary" icon={<Plus className="h-4 w-4" />} onClick={() => openEditor(null)}>
                      Create ruleset
                    </Button>
                  }
                />
              </div>
            )}

            {isEditorOpen && (
              <SectionCard title={editingId ? 'Edit ruleset' : 'New ruleset'}>
                <form
                  className="space-y-4"
                  onSubmit={(e) => {
                    e.preventDefault();
                    if (!jsonError) saveTransform.mutate();
                  }}
                >
                  {!enabled && (
                    <Notice tone="info">
                      This ruleset is currently <strong>disabled</strong>. Incoming traffic will pass through untouched until enabled.
                    </Notice>
                  )}

                  <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                    <Field
                      label="Ruleset name"
                      value={transformName}
                      onChange={(e) => setTransformName(e.target.value)}
                      placeholder="e.g. OGC features link rewrite"
                      required
                    />
                    <Field
                      label="Match path"
                      value={matchPath}
                      onChange={(e) => setMatchPath(e.target.value)}
                      placeholder="/*"
                      inputClassName="font-mono"
                      helper="Use /* to match every route on this service"
                      required
                    />
                  </div>

                  <Field
                    label="Description"
                    value={transformDescription}
                    onChange={(e) => setTransformDescription(e.target.value)}
                    placeholder="Rewrite self-referencing links in responses"
                  />

                  <div className="rounded-control border border-rule bg-paper-2 p-3">
                    <label className="flex cursor-pointer items-center justify-between gap-3">
                      <div>
                        <span className="block text-sm font-medium text-ink">Ruleset enabled</span>
                        <span className="block text-xs text-muted">
                          When enabled, responses matching this path will be rewritten by the rules below.
                        </span>
                      </div>
                      <input
                        type="checkbox"
                        checked={enabled}
                        onChange={(e) => setEnabled(e.target.checked)}
                        className="h-4 w-4 accent-accent"
                      />
                    </label>
                  </div>

                  <div>
                    <label htmlFor="rules-json" className="ui-field__label">
                      Rules JSON
                    </label>
                    <div className="mb-2 flex flex-wrap items-center gap-1.5 text-xs text-muted">
                      <span>Insert variable:</span>
                      {[
                        { label: '{{oryca_gateway_url}}', tip: 'Public gateway base URL' },
                        { label: '{{oryca_auth}}', tip: 'Caller API key or Bearer token query param' },
                      ].map((v) => (
                        <button
                          key={v.label}
                          type="button"
                          onClick={() => {
                            navigator.clipboard.writeText(v.label);
                            toast({ tone: 'ok', message: `Copied ${v.label} to clipboard` });
                          }}
                          title={v.tip}
                          className="inline-flex items-center gap-1 rounded-chip border border-rule bg-paper-3 px-2 py-0.5 font-mono text-2xs text-ink-2 transition-colors hover:border-accent-edge hover:bg-accent-wash hover:text-accent"
                        >
                          {v.label}
                        </button>
                      ))}
                    </div>

                    {jsonError && (
                      <Notice tone="danger" className="mb-2">
                        {jsonError}
                      </Notice>
                    )}

                    <textarea
                      id="rules-json"
                      rows={14}
                      required
                      aria-invalid={jsonError ? true : undefined}
                      value={rulesJson}
                      onChange={(e) => setRulesJson(e.target.value)}
                      className="ui-input w-full bg-graphite p-4 font-mono text-xs text-graphite-ink"
                    />
                  </div>

                  <div className="flex flex-wrap justify-end gap-2 border-t border-rule pt-4">
                    <Button onClick={closeEditor}>Cancel</Button>
                    <Button
                      type="submit"
                      variant="primary"
                      loading={saveTransform.isPending}
                      disabled={Boolean(jsonError)}
                    >
                      Save ruleset
                    </Button>
                  </div>
                </form>
              </SectionCard>
            )}
          </div>
        </div>
      )}
    </NavigationShell>
  );
}
