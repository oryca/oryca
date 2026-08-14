'use client';

import React, { useState, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import NavigationShell from '@/components/NavigationShell';
import {
  Layers,
  Server,
  Sparkles,
  Plus,
  Trash2,
  Edit,
  Save,
  CheckCircle,
  AlertTriangle,
  ChevronDown,
  Info,
  Sliders,
  Code
} from 'lucide-react';

interface TransformConfig {
  id?: string;
  name: string;
  description?: string;
  serviceId: string;
  match: {
    path: string;
    methods?: string[];
    options?: Record<string, any>;
  };
  enabled: boolean;
  rules: any[];
}

interface Preset {
  name: string;
  types: string[];
  title: string;
  description: string;
  match: any;
  rules: any[];
}

export default function AdminTransformsPage() {
  const queryClient = useQueryClient();
  const [selectedServiceId, setSelectedServiceId] = useState('');
  
  const [transformName, setTransformName] = useState('');
  const [transformDescription, setTransformDescription] = useState('');
  const [matchPath, setMatchPath] = useState('/*');
  const [matchMethods, setMatchMethods] = useState<string[]>(['GET']);
  const [enabled, setEnabled] = useState(true);
  const [rulesJson, setRulesJson] = useState('[]');

  const [activeTransform, setActiveTransform] = useState<TransformConfig | null>(null);
  const [isEditorOpen, setIsEditorOpen] = useState(false);
  const [success, setSuccess] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Fetch Services
  const { data: services } = useQuery<any[]>({
    queryKey: ['admin-transform-services'],
    queryFn: async () => {
      const res = await api.get('/services');
      return res.data.items || [];
    },
  });

  const selectedService = services?.find((s) => s.id === selectedServiceId);

  // Fetch Response Transforms for selected service
  const { data: transforms, isLoading: isLoadingTransforms, refetch: refetchTransforms } = useQuery<any>({
    queryKey: ['transforms-for-service', selectedServiceId],
    queryFn: async () => {
      if (!selectedServiceId) return null;
      const res = await api.get(`/response-transforms?serviceId=${selectedServiceId}`);
      // The endpoint returns `{ numberMatched, numberReturned, items }`
      return res.data.items || [];
    },
    enabled: !!selectedServiceId,
  });

  // Fetch presets for selected service type
  const { data: presets } = useQuery<Preset[]>({
    queryKey: ['presets-for-type', selectedService?.type],
    queryFn: async () => {
      if (!selectedService?.type) return [];
      const res = await api.get(`/response-transforms/presets?type=${selectedService.type}`);
      return res.data || [];
    },
    enabled: !!selectedService?.type,
  });

  // Load selected transform into form states
  useEffect(() => {
    if (activeTransform) {
      setTransformName(activeTransform.name || '');
      setTransformDescription(activeTransform.description || '');
      setMatchPath(activeTransform.match?.path || '/*');
      setMatchMethods(activeTransform.match?.methods || ['GET']);
      setEnabled(activeTransform.enabled !== false);
      setRulesJson(JSON.stringify(activeTransform.rules || [], null, 2));
    } else {
      setTransformName('');
      setTransformDescription('');
      setMatchPath('/*');
      setMatchMethods(['GET']);
      setEnabled(true);
      setRulesJson('[]');
    }
    setSuccess(null);
    setError(null);
  }, [activeTransform]);

  // Handle auto-select first service
  useEffect(() => {
    if (services && services.length > 0 && !selectedServiceId) {
      setSelectedServiceId(services[0].id);
    }
  }, [services, selectedServiceId]);

  // Mutations
  const applyPresetMutation = useMutation({
    mutationFn: async (presetName: string) => {
      setSuccess(null);
      setError(null);
      const res = await api.post(`/response-transforms/presets/${presetName}/apply`, {
        serviceId: selectedServiceId,
      });
      return res.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['transforms-for-service', selectedServiceId] });
      setSuccess('Preset rules applied successfully.');
    },
    onError: (err: any) => {
      setError(err.message || 'Failed to apply preset transform.');
    },
  });

  const saveTransformMutation = useMutation({
    mutationFn: async () => {
      setSuccess(null);
      setError(null);

      let parsedRules = [];
      try {
        parsedRules = JSON.parse(rulesJson);
        if (!Array.isArray(parsedRules)) {
          throw new Error('Rules must be an array.');
        }
      } catch (err: any) {
        throw new Error(`Invalid JSON format in Rules: ${err.message}`);
      }

      const payload = {
        name: transformName,
        description: transformDescription,
        serviceId: selectedServiceId,
        match: {
          path: matchPath,
          methods: matchMethods,
        },
        enabled,
        rules: parsedRules,
      };

      if (activeTransform?.id) {
        await api.put(`/response-transforms/${activeTransform.id}`, payload);
      } else {
        await api.post('/response-transforms', payload);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['transforms-for-service', selectedServiceId] });
      setSuccess('Response transform configuration saved.');
      setIsEditorOpen(false);
      setActiveTransform(null);
    },
    onError: (err: any) => {
      setError(err.message || 'Failed to save transform.');
    },
  });

  const deleteTransformMutation = useMutation({
    mutationFn: async (id: string) => {
      await api.delete(`/response-transforms/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['transforms-for-service', selectedServiceId] });
      setSuccess('Response transform configuration deleted.');
    },
  });

  return (
    <NavigationShell>
      <div className="space-y-6">
        {/* Service select filter */}
        <div className="bg-paper p-4 border border-rule rounded-surface shadow-sm flex flex-wrap gap-4 items-center justify-between">
          <div className="flex items-center gap-2">
            <Server className="w-5 h-5 text-accent" />
            <div>
              <p className="text-xs font-semibold text-muted uppercase tracking-wider">Select API Service</p>
              <p className="text-xs text-ink-3">Choose the service to configure link-rewrite rules for</p>
            </div>
          </div>

          <div className="min-w-[240px] max-w-sm relative">
            <select
              value={selectedServiceId}
              onChange={(e) => {
                setSelectedServiceId(e.target.value);
                setActiveTransform(null);
                setIsEditorOpen(false);
              }}
              className="w-full bg-paper-2 border border-rule rounded-control px-3 py-1.5 text-xs text-ink outline-none appearance-none focus:border-focus"
            >
              {services?.map((svc) => (
                <option key={svc.id} value={svc.id}>
                  {svc.name} ({svc.basePath})
                </option>
              ))}
            </select>
            <ChevronDown className="w-3.5 h-3.5 text-muted absolute right-3 top-2.5 pointer-events-none" />
          </div>
        </div>

        {success && (
          <div className="flex items-start gap-3 rounded-control border border-ok-edge bg-ok-wash p-4 text-xs text-ok">
            <CheckCircle className="w-4 h-4 mt-0.5 flex-shrink-0" />
            <div>{success}</div>
          </div>
        )}

        {error && (
          <div className="flex items-start gap-3 rounded-control border border-danger-edge bg-danger-wash p-4 text-xs text-danger">
            <AlertTriangle className="w-4 h-4 mt-0.5 flex-shrink-0" />
            <div>{error}</div>
          </div>
        )}

        {/* Main Work Area */}
        {selectedServiceId && (
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
            {/* Left section: Active Transforms list & presets */}
            <div className="lg:col-span-1 space-y-6">
              {/* Presets Offer Card */}
              {presets && presets.length > 0 && (!transforms || transforms.length === 0) && (
                <div className="bg-accent-wash border-2 border-accent rounded-surface p-6 shadow-sm relative overflow-hidden">
                  <div className="absolute right-4 top-4 text-accent opacity-15">
                    <Sparkles className="w-12 h-12" />
                  </div>
                  <h3 className="font-title text-base font-bold text-accent flex items-center gap-2 mb-2">
                    <Sparkles className="w-5 h-5" /> Rewrite Upstream Links?
                  </h3>
                  <p className="text-xs text-ink-2 mb-4">
                    This geospatial OGC API responds with links pointing directly at the upstream server. Applying a rewrite preset keeps clients routed on the gateway.
                  </p>

                  <div className="space-y-2">
                    {presets.map((preset) => (
                      <button
                        key={preset.name}
                        onClick={() => applyPresetMutation.mutate(preset.name)}
                        disabled={applyPresetMutation.isPending}
                        className="w-full text-left p-3 bg-paper border border-accent-edge hover:border-accent rounded-control transition text-xs font-semibold text-accent flex items-center justify-between"
                      >
                        <div>
                          <p>{preset.title}</p>
                          <p className="text-[10px] text-muted font-normal mt-0.5">{preset.description}</p>
                        </div>
                        <Plus className="w-4 h-4 flex-shrink-0 ml-2" />
                      </button>
                    ))}
                  </div>
                </div>
              )}

              {/* Active list */}
              <div className="bg-paper border border-rule rounded-surface p-6 shadow-sm space-y-4">
                <div className="flex items-center justify-between">
                  <h3 className="font-title text-sm font-semibold text-ink">Active Rulesets</h3>
                  <button
                    onClick={() => {
                      setActiveTransform(null);
                      setIsEditorOpen(true);
                    }}
                    className="text-xs text-accent font-semibold hover:underline"
                  >
                    + Create custom
                  </button>
                </div>

                {isLoadingTransforms ? (
                  <div className="h-16 bg-paper-2 rounded-control animate-pulse"></div>
                ) : !transforms || transforms.length === 0 ? (
                  <p className="text-xs text-muted py-4 text-center">No rewrite rules defined.</p>
                ) : (
                  <div className="space-y-2">
                    {transforms.map((t: any) => (
                      <div
                        key={t.id}
                        className={`p-3 rounded-control border flex items-center justify-between gap-2 ${
                          activeTransform?.id === t.id
                            ? 'bg-accent-wash border-accent-edge text-ink'
                            : 'bg-paper-2 border-rule text-ink-2'
                        }`}
                      >
                        <div className="min-w-0">
                          <p className="text-xs font-semibold truncate">{t.name}</p>
                          <p className="text-[10px] text-muted font-mono truncate">Match: {t.match?.path}</p>
                        </div>
                        <div className="flex gap-1">
                          <button
                            onClick={() => {
                              setActiveTransform(t);
                              setIsEditorOpen(true);
                            }}
                            className="p-1 text-muted hover:text-ink rounded-control"
                          >
                            <Edit className="w-3.5 h-3.5" />
                          </button>
                          <button
                            onClick={() => {
                              if (confirm(`Delete ruleset "${t.name}"?`)) {
                                deleteTransformMutation.mutate(t.id);
                              }
                            }}
                            className="p-1 text-muted hover:text-danger rounded-control"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>

            {/* Right section: Form Rules Editor */}
            <div className="lg:col-span-2">
              {isEditorOpen ? (
                <div className="bg-paper border border-rule rounded-surface p-6 shadow-sm space-y-4">
                  <h3 className="font-title text-base font-bold text-ink border-b border-rule pb-2">
                    {activeTransform ? 'Edit Transform Configuration' : 'Create Custom Transform'}
                  </h3>

                  <form
                    onSubmit={(e) => {
                      e.preventDefault();
                      saveTransformMutation.mutate();
                    }}
                    className="space-y-4 text-xs"
                  >
                    <div className="grid grid-cols-2 gap-4">
                      <div>
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          Ruleset Name
                        </label>
                        <input
                          type="text"
                          required
                          value={transformName}
                          onChange={(e) => setTransformName(e.target.value)}
                          placeholder="e.g. OGC features link rewrite"
                          className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                        />
                      </div>
                      <div>
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          Match Path
                        </label>
                        <input
                          type="text"
                          required
                          value={matchPath}
                          onChange={(e) => setMatchPath(e.target.value)}
                          placeholder="/*"
                          className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus font-mono"
                        />
                      </div>
                    </div>

                    <div>
                      <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                        Description
                      </label>
                      <input
                        type="text"
                        value={transformDescription}
                        onChange={(e) => setTransformDescription(e.target.value)}
                        placeholder="Rewrite self-referencing links in responses"
                        className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                      />
                    </div>

                    <div className="grid grid-cols-2 gap-4 items-center">
                      <div className="flex items-center gap-2 pt-2">
                        <input
                          type="checkbox"
                          id="transform-enabled"
                          checked={enabled}
                          onChange={(e) => setEnabled(e.target.checked)}
                          className="w-4 h-4 accent-accent"
                        />
                        <label htmlFor="transform-enabled" className="text-xs font-semibold text-ink-2 select-none cursor-pointer">
                          Ruleset Enabled
                        </label>
                      </div>
                    </div>

                    {/* Rules spec text area */}
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted uppercase tracking-wider block">
                        Rewrite Rules Spec (JSON Array)
                      </label>
                      <div className="bg-paper-2 p-3 border border-rule rounded-control text-[10px] text-muted flex gap-2">
                        <Info className="w-4 h-4 text-accent mt-0.5 flex-shrink-0" />
                        <p>
                          Define rewrite actions (replace strings, regular expressions, xpath mutations) inside the JSON rules array. Use variables like <code className="bg-paper-3 px-1 py-0.5 rounded font-mono">{"{{sourceUrl}}"}</code>.
                        </p>
                      </div>
                      <textarea
                        rows={12}
                        required
                        value={rulesJson}
                        onChange={(e) => setRulesJson(e.target.value)}
                        className="w-full bg-graphite text-graphite-ink border border-graphite-rule rounded-control p-4 font-mono text-[11px] outline-none focus:border-accent"
                      />
                    </div>

                    <div className="flex justify-end gap-3 pt-2">
                      <button
                        type="button"
                        onClick={() => {
                          setIsEditorOpen(false);
                          setActiveTransform(null);
                        }}
                        className="px-4 py-2 border border-rule hover:border-faint rounded-control text-ink-2 hover:bg-paper-2 transition"
                      >
                        Cancel
                      </button>
                      <button
                        type="submit"
                        disabled={saveTransformMutation.isPending}
                        className="px-4 py-2 bg-accent hover:bg-accent-deep text-accent-ink font-semibold rounded-control transition"
                      >
                        {saveTransformMutation.isPending ? 'Saving...' : 'Save Ruleset'}
                      </button>
                    </div>
                  </form>
                </div>
              ) : (
                <div className="h-64 bg-paper border border-rule border-dashed rounded-surface flex flex-col items-center justify-center text-muted p-8 text-center gap-2 shadow-sm">
                  <Sliders className="w-8 h-8 text-muted opacity-50" />
                  <p className="text-xs">Select or create a ruleset to edit response transform rules.</p>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </NavigationShell>
  );
}
