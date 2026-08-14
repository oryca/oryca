'use client';

import React, { useState, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import NavigationShell from '@/components/NavigationShell';
import Link from 'next/link';
import {
  Database,
  Server,
  Plus,
  Trash2,
  Edit,
  CheckCircle,
  AlertTriangle,
  ArrowRight,
  Sparkles,
  RefreshCw,
  Eye,
  Settings,
  HelpCircle,
  BookOpen
} from 'lucide-react';

interface GatewaySource {
  id: string;
  alias: string;
  name: string;
  description?: string;
  type: string; // upstream | static
  protocol?: string; // http | https
  url?: string;
  headers?: Array<{ key: string; value: string }>;
  contentType?: string;
  body?: string;
}

interface ResourcePath {
  path: string;
  methods: string[];
  sourceAlias: string;
}

interface GatewayService {
  id: string;
  name: string;
  description?: string;
  type: string; // General | OGC_API_Features | OGC_API_STAC | OGC_API_Styles | OGC_API_SensorThings | OGC_API_Tiles
  basePath: string;
  enabled?: boolean;
  isPublic?: boolean;
  resourcePaths?: ResourcePath[];
}

export default function AdminServicesPage() {
  const queryClient = useQueryClient();
  const [activeTab, setActiveTab] = useState<'services' | 'sources'>('services');

  // Form toggles
  const [editingSource, setEditingSource] = useState<GatewaySource | null>(null);
  const [isSourceFormOpen, setIsSourceFormOpen] = useState(false);
  const [editingService, setEditingService] = useState<GatewayService | null>(null);
  const [isServiceFormOpen, setIsServiceFormOpen] = useState(false);

  // Sources query
  const { data: sources, isLoading: isLoadingSources } = useQuery<GatewaySource[]>({
    queryKey: ['admin-sources'],
    queryFn: async () => {
      const res = await api.get('/sources');
      return res.data.items || [];
    },
  });

  // Services query
  const { data: services, isLoading: isLoadingServices } = useQuery<GatewayService[]>({
    queryKey: ['admin-services'],
    queryFn: async () => {
      const res = await api.get('/services');
      return res.data.items || [];
    },
  });

  return (
    <NavigationShell>
      <div className="space-y-6">
        <div>
          <h1 className="font-title text-2xl font-bold tracking-tight text-ink">
            Gateway Administration
          </h1>
          <p className="text-sm text-muted">
            Configure upstream sources, register API routes, and deploy response transform rules
          </p>
        </div>

        {/* Tab selector */}
        <div className="flex border-b border-rule pb-px gap-6">
          <button
            onClick={() => setActiveTab('services')}
            className={`pb-3 text-sm font-semibold border-b-2 transition ${
              activeTab === 'services'
                ? 'border-accent text-ink font-bold'
                : 'border-transparent text-muted hover:text-ink'
            }`}
          >
            📡 Published Services
          </button>
          <button
            onClick={() => setActiveTab('sources')}
            className={`pb-3 text-sm font-semibold border-b-2 transition ${
              activeTab === 'sources'
                ? 'border-accent text-ink font-bold'
                : 'border-transparent text-muted hover:text-ink'
            }`}
          >
            📦 Upstream Sources
          </button>
        </div>

        {activeTab === 'sources' ? (
          <SourcesManager
            sources={sources || []}
            isLoading={isLoadingSources}
            isOpen={isSourceFormOpen}
            setIsOpen={setIsSourceFormOpen}
            editingSource={editingSource}
            setEditingSource={setEditingSource}
            queryClient={queryClient}
          />
        ) : (
          <ServicesManager
            services={services || []}
            sources={sources || []}
            isLoading={isLoadingServices}
            isOpen={isServiceFormOpen}
            setIsOpen={setIsServiceFormOpen}
            editingService={editingService}
            setEditingService={setEditingService}
            queryClient={queryClient}
          />
        )}
      </div>
    </NavigationShell>
  );
}

// ================================= SOURCES SUB-COMPONENT =================================
function SourcesManager({
  sources,
  isLoading,
  isOpen,
  setIsOpen,
  editingSource,
  setEditingSource,
  queryClient
}: {
  sources: GatewaySource[];
  isLoading: boolean;
  isOpen: boolean;
  setIsOpen: (val: boolean) => void;
  editingSource: GatewaySource | null;
  setEditingSource: (src: GatewaySource | null) => void;
  queryClient: any;
}) {
  const [alias, setAlias] = useState('');
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [type, setType] = useState('api'); // upstream | static
  const [protocol, setProtocol] = useState('https');
  const [url, setUrl] = useState('');
  const [contentType, setContentType] = useState('application/json');
  const [body, setBody] = useState('');

  const [formError, setFormError] = useState<string | null>(null);

  useEffect(() => {
    if (editingSource) {
      setAlias(editingSource.alias || '');
      setName(editingSource.name || '');
      setDescription(editingSource.description || '');
      setType(editingSource.type || 'api');
      setProtocol(editingSource.protocol || 'https');
      setUrl(editingSource.url || '');
      setContentType(editingSource.contentType || 'application/json');
      setBody(editingSource.body || '');
      setIsOpen(true);
    } else {
      setAlias('');
      setName('');
      setDescription('');
      setType('api');
      setProtocol('https');
      setUrl('');
      setContentType('application/json');
      setBody('');
    }
  }, [editingSource, isOpen, setIsOpen]);

  const saveMutation = useMutation({
    mutationFn: async () => {
      setFormError(null);
      const payload = {
        alias,
        name,
        description,
        type,
        protocol,
        url,
        contentType,
        body: type === 'static' ? body : undefined,
      };

      if (editingSource) {
        await api.put(`/sources/${editingSource.id}`, payload);
      } else {
        await api.post('/sources', payload);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-sources'] });
      setIsOpen(false);
      setEditingSource(null);
    },
    onError: (err: any) => {
      setFormError(err.message || 'Failed to save source.');
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async (id: string) => {
      await api.delete(`/sources/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-sources'] });
    },
  });

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h2 className="text-xs font-bold text-muted uppercase tracking-wider">Configure Upstream Targets</h2>
        <button
          onClick={() => {
            setEditingSource(null);
            setIsOpen(true);
          }}
          className="px-3 py-1.5 bg-accent hover:bg-accent-deep text-accent-ink text-xs font-semibold rounded-control transition duration-short flex items-center gap-1"
        >
          <Plus className="w-4 h-4" /> Add Source
        </button>
      </div>

      {isOpen && (
        <div className="fixed inset-0 z-50 bg-scrim flex items-center justify-center p-4">
          <div className="bg-paper border border-rule rounded-surface p-6 w-full max-w-lg shadow-sm space-y-4 max-h-[90vh] overflow-y-auto">
            <h3 className="font-title text-base font-bold text-ink">
              {editingSource ? 'Edit Upstream Source' : 'Add Upstream Source'}
            </h3>

            {formError && (
              <div className="flex items-start gap-3 rounded-control border border-danger-edge bg-danger-wash p-4 text-xs text-danger">
                <AlertTriangle className="w-4 h-4 mt-0.5 flex-shrink-0" />
                <div>{formError}</div>
              </div>
            )}

            <form
              onSubmit={(e) => {
                e.preventDefault();
                saveMutation.mutate();
              }}
              className="space-y-4 text-xs"
            >
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                    Alias (Unique key)
                  </label>
                  <input
                    type="text"
                    required
                    value={alias}
                    onChange={(e) => setAlias(e.target.value)}
                    placeholder="smoke-upstream"
                    disabled={!!editingSource}
                    className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus font-mono"
                  />
                </div>
                <div>
                  <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                    Display Name
                  </label>
                  <input
                    type="text"
                    required
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="My Production API"
                    className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                  />
                </div>
              </div>

              <div>
                <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                  Description
                </label>
                <input
                  type="text"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Optional context about the source"
                  className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                />
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                    Source Type
                  </label>
                  <select
                    value={type}
                    onChange={(e) => setType(e.target.value)}
                    className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                  >
                    <option value="api">Proxy Upstream URL</option>
                    <option value="static">Static JSON Content</option>
                  </select>
                </div>
                {type === 'api' && (
                  <div>
                    <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                      Protocol
                    </label>
                    <select
                      value={protocol}
                      onChange={(e) => setProtocol(e.target.value)}
                      className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                    >
                      <option value="https">HTTPS</option>
                      <option value="http">HTTP</option>
                    </select>
                  </div>
                )}
              </div>

              {type === 'api' ? (
                <div>
                  <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                    Target Base URL
                  </label>
                  <input
                    type="text"
                    required
                    value={url}
                    onChange={(e) => setUrl(e.target.value)}
                    placeholder="https://httpbin.org (without trailing slash)"
                    className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus font-mono"
                  />
                </div>
              ) : (
                <div className="space-y-4">
                  <div>
                    <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                      Content Type
                    </label>
                    <input
                      type="text"
                      required
                      value={contentType}
                      onChange={(e) => setContentType(e.target.value)}
                      placeholder="application/json"
                      className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus font-mono"
                    />
                  </div>
                  <div>
                    <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                      Static Payload Body
                    </label>
                    <textarea
                      rows={5}
                      value={body}
                      onChange={(e) => setBody(e.target.value)}
                      placeholder='{ "status": "ok" }'
                      className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus font-mono"
                    />
                  </div>
                </div>
              )}

              <div className="flex justify-end gap-3 pt-2">
                <button
                  type="button"
                  onClick={() => {
                    setIsOpen(false);
                    setEditingSource(null);
                  }}
                  className="px-4 py-2 border border-rule hover:border-faint rounded-control text-ink-2 hover:bg-paper-2 transition"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={saveMutation.isPending}
                  className="px-4 py-2 bg-accent hover:bg-accent-deep text-accent-ink font-semibold rounded-control transition"
                >
                  {saveMutation.isPending ? 'Saving...' : 'Save Source'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {isLoading ? (
        <div className="space-y-4">
          {[...Array(2)].map((_, i) => (
            <div key={i} className="h-16 bg-paper rounded-surface border border-rule animate-pulse"></div>
          ))}
        </div>
      ) : sources.length === 0 ? (
        <div className="p-8 bg-paper border border-rule border-dashed rounded-surface text-center text-xs text-muted">
          No upstream sources configured. Configure one to link routing paths to.
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {sources.map((src) => (
            <div key={src.id} className="bg-paper border border-rule rounded-surface p-4 shadow-sm hover:border-faint transition flex flex-col justify-between h-40">
              <div>
                <div className="flex items-center justify-between gap-2">
                  <div className="flex items-center gap-2">
                    <Database className="w-4 h-4 text-accent" />
                    <span className="font-title text-sm font-semibold text-ink">{src.name}</span>
                  </div>
                  <span className="text-[10px] font-mono px-2 py-0.5 rounded-chip bg-accent-wash border border-accent-edge text-accent">
                    Alias: {src.alias}
                  </span>
                </div>
                <p className="text-xs text-muted mt-2 line-clamp-2">{src.description || 'No description.'}</p>
                <div className="mt-3 text-[10px] font-mono text-muted truncate">
                  {src.type === 'api' ? `Proxy: ${src.url}` : `Static Content (${src.contentType})`}
                </div>
              </div>

              <div className="flex justify-end gap-2 border-t border-rule pt-2 mt-2">
                <button
                  onClick={() => setEditingSource(src)}
                  className="p-1.5 rounded-control text-muted hover:text-ink hover:bg-paper-3 transition"
                  title="Edit Source"
                >
                  <Edit className="w-4 h-4" />
                </button>
                <button
                  onClick={() => {
                    if (confirm(`Delete upstream source "${src.name}"?`)) {
                      deleteMutation.mutate(src.id);
                    }
                  }}
                  disabled={deleteMutation.isPending}
                  className="p-1.5 rounded-control text-muted hover:text-danger hover:bg-danger-wash border border-transparent hover:border-danger-edge transition"
                  title="Delete Source"
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// ================================= SERVICES SUB-COMPONENT =================================
function ServicesManager({
  services,
  sources,
  isLoading,
  isOpen,
  setIsOpen,
  editingService,
  setEditingService,
  queryClient
}: {
  services: GatewayService[];
  sources: GatewaySource[];
  isLoading: boolean;
  isOpen: boolean;
  setIsOpen: (val: boolean) => void;
  editingService: GatewayService | null;
  setEditingService: (svc: GatewayService | null) => void;
  queryClient: any;
}) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [type, setType] = useState('General');
  const [basePath, setBasePath] = useState('');
  const [enabled, setEnabled] = useState(true);
  const [isPublic, setIsPublic] = useState(false);
  const [resourcePaths, setResourcePaths] = useState<ResourcePath[]>([
    { path: '/', methods: ['GET'], sourceAlias: '' },
  ]);

  const [formError, setFormError] = useState<string | null>(null);
  const [scaffoldSuggestions, setScaffoldSuggestions] = useState<string[]>([]);
  const [isScaffoldingLoading, setIsScaffoldingLoading] = useState(false);

  useEffect(() => {
    if (editingService) {
      setName(editingService.name || '');
      setDescription(editingService.description || '');
      setType(editingService.type || 'General');
      setBasePath(editingService.basePath || '');
      setEnabled(editingService.enabled !== false);
      setIsPublic(editingService.isPublic === true);
      setResourcePaths(
        editingService.resourcePaths && editingService.resourcePaths.length > 0
          ? editingService.resourcePaths.map((rp) => ({ ...rp }))
          : [{ path: '/', methods: ['GET'], sourceAlias: '' }]
      );
      setIsOpen(true);
    } else {
      setName('');
      setDescription('');
      setType('General');
      setBasePath('');
      setEnabled(true);
      setIsPublic(false);
      setResourcePaths([{ path: '/', methods: ['GET'], sourceAlias: '' }]);
    }
    setScaffoldSuggestions([]);
  }, [editingService, isOpen, setIsOpen]);

  // Scaffolding when service type changes (only OGC types)
  useEffect(() => {
    if (!basePath || type === 'General') {
      setScaffoldSuggestions([]);
      return;
    }

    async function checkPaths() {
      setIsScaffoldingLoading(true);
      try {
        const pathsPayload = {
          type,
          basePath,
          resourcePaths: resourcePaths.map((r) => r.path),
        };
        const res = await api.post('/services/check-paths', pathsPayload);
        if (res.data.missingOgcPaths) {
          setScaffoldSuggestions(res.data.missingOgcPaths);
        }
      } catch (err) {
        console.error('Failed path check:', err);
      } finally {
        setIsScaffoldingLoading(false);
      }
    }

    // Debounce calls during typing
    const delay = setTimeout(checkPaths, 800);
    return () => clearTimeout(delay);
  }, [type, basePath, resourcePaths]);

  const addPathRow = (pathValue = '/') => {
    setResourcePaths((prev) => [
      ...prev,
      { path: pathValue, methods: ['GET'], sourceAlias: sources[0]?.alias || '' },
    ]);
  };

  const removePathRow = (index: number) => {
    setResourcePaths((prev) => prev.filter((_, i) => i !== index));
  };

  const updatePathRow = (index: number, field: keyof ResourcePath, value: any) => {
    setResourcePaths((prev) =>
      prev.map((rp, i) => (i === index ? { ...rp, [field]: value } : rp))
    );
  };

  const saveMutation = useMutation({
    mutationFn: async () => {
      setFormError(null);
      // Validate mapping aliases
      if (resourcePaths.some((r) => !r.sourceAlias)) {
        throw new Error('All resource paths must be mapped to an upstream source.');
      }
      if (!basePath.startsWith('/')) {
        throw new Error('Base Path must start with /');
      }

      const payload = {
        name,
        description,
        type,
        basePath,
        enabled,
        isPublic,
        resourcePaths,
      };

      if (editingService) {
        await api.put(`/services/${editingService.id}`, payload);
      } else {
        await api.post('/services', payload);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-services'] });
      setIsOpen(false);
      setEditingService(null);
    },
    onError: (err: any) => {
      setFormError(err.message || 'Failed to save service.');
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async (id: string) => {
      await api.delete(`/services/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-services'] });
    },
  });

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h2 className="text-xs font-bold text-muted uppercase tracking-wider">Publish Routing Gateways</h2>
        <button
          onClick={() => {
            setEditingService(null);
            setIsOpen(true);
          }}
          className="px-3 py-1.5 bg-accent hover:bg-accent-deep text-accent-ink text-xs font-semibold rounded-control transition duration-short flex items-center gap-1"
        >
          <Plus className="w-4 h-4" /> Publish Service
        </button>
      </div>

      {isOpen && (
        <div className="fixed inset-0 z-50 bg-scrim flex items-center justify-center p-4">
          <div className="bg-paper border border-rule rounded-surface p-6 w-full max-w-2xl shadow-sm space-y-4 max-h-[95vh] overflow-y-auto">
            <h3 className="font-title text-base font-bold text-ink">
              {editingService ? 'Edit Gateway Service' : 'Publish New Service'}
            </h3>

            {formError && (
              <div className="flex items-start gap-3 rounded-control border border-danger-edge bg-danger-wash p-4 text-xs text-danger">
                <AlertTriangle className="w-4 h-4 mt-0.5 flex-shrink-0" />
                <div>{formError}</div>
              </div>
            )}

            <form
              onSubmit={(e) => {
                e.preventDefault();
                saveMutation.mutate();
              }}
              className="space-y-4 text-xs"
            >
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                    Service Name
                  </label>
                  <input
                    type="text"
                    required
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="e.g. STAC Features API"
                    className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                  />
                </div>
                <div>
                  <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                    Base Path (Prefix route)
                  </label>
                  <input
                    type="text"
                    required
                    value={basePath}
                    onChange={(e) => setBasePath(e.target.value)}
                    placeholder="/my-service"
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
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Geospatial STAC server description"
                  className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                />
              </div>

              <div className="grid grid-cols-3 gap-4 items-center">
                <div>
                  <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                    Service Type
                  </label>
                  <select
                    value={type}
                    onChange={(e) => setType(e.target.value)}
                    className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                  >
                    <option value="General">General API</option>
                    <option value="OGC_API_Features">OGC API Features</option>
                    <option value="OGC_API_STAC">OGC API STAC</option>
                    <option value="OGC_API_Styles">OGC API Styles</option>
                    <option value="OGC_API_SensorThings">OGC API SensorThings</option>
                    <option value="OGC_API_Tiles">OGC API Tiles</option>
                  </select>
                </div>

                <div className="flex items-center gap-2 pt-5">
                  <input
                    type="checkbox"
                    id="svc-enabled"
                    checked={enabled}
                    onChange={(e) => setEnabled(e.target.checked)}
                    className="w-4 h-4 accent-accent"
                  />
                  <label htmlFor="svc-enabled" className="text-xs font-semibold text-ink-2 select-none cursor-pointer">
                    Enable Routing
                  </label>
                </div>

                <div className="flex items-center gap-2 pt-5">
                  <input
                    type="checkbox"
                    id="svc-public"
                    checked={isPublic}
                    onChange={(e) => setIsPublic(e.target.checked)}
                    className="w-4 h-4 accent-accent"
                  />
                  <label htmlFor="svc-public" className="text-xs font-semibold text-ink-2 select-none cursor-pointer">
                    Public access (no API key)
                  </label>
                </div>
              </div>

              {/* OGC Path scaffolding notifications */}
              {scaffoldSuggestions.length > 0 && (
                <div className="bg-accent-wash border border-accent-edge p-4 rounded-control space-y-2">
                  <div className="flex items-center gap-1.5 text-accent font-semibold">
                    <Sparkles className="w-4 h-4" /> Recommended OGC Paths
                  </div>
                  <p className="text-[10px] text-muted">
                    This service standard recommends implementing the following resource paths:
                  </p>
                  <div className="flex flex-wrap gap-2">
                    {scaffoldSuggestions.map((path) => (
                      <button
                        key={path}
                        type="button"
                        onClick={() => addPathRow(path)}
                        className="px-2.5 py-1 bg-paper hover:bg-paper-3 border border-rule text-[10px] font-mono rounded-chip text-accent flex items-center gap-1"
                      >
                        + {path}
                      </button>
                    ))}
                  </div>
                </div>
              )}

              {/* Resource paths editor mapping */}
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <label className="text-xs font-semibold text-muted uppercase tracking-wider">
                    Resource Paths & Target Sources
                  </label>
                  <button
                    type="button"
                    onClick={() => addPathRow()}
                    className="text-xs text-accent font-semibold hover:underline"
                  >
                    + Add Path Row
                  </button>
                </div>

                <div className="space-y-2 max-h-52 overflow-y-auto pr-1">
                  {resourcePaths.map((rp, index) => (
                    <div key={index} className="flex gap-2 items-center bg-paper-2 p-2 border border-rule rounded-control">
                      <div className="flex-[2] min-w-0">
                        <input
                          type="text"
                          required
                          value={rp.path}
                          onChange={(e) => updatePathRow(index, 'path', e.target.value)}
                          placeholder="/collections/*"
                          className="w-full bg-paper border border-rule rounded-control px-2.5 py-1 text-xs text-ink outline-none focus:border-focus font-mono"
                        />
                      </div>

                      <div className="flex-1">
                        <select
                          value={rp.methods[0] || 'GET'}
                          onChange={(e) => updatePathRow(index, 'methods', [e.target.value])}
                          className="w-full bg-paper border border-rule rounded-control px-2.5 py-1 text-xs text-ink outline-none"
                        >
                          <option value="GET">GET</option>
                          <option value="POST">POST</option>
                          <option value="PUT">PUT</option>
                          <option value="DELETE">DELETE</option>
                        </select>
                      </div>

                      <div className="flex-[2]">
                        <select
                          required
                          value={rp.sourceAlias}
                          onChange={(e) => updatePathRow(index, 'sourceAlias', e.target.value)}
                          className="w-full bg-paper border border-rule rounded-control px-2.5 py-1 text-xs text-ink outline-none"
                        >
                          <option value="">Map upstream...</option>
                          {sources.map((src) => (
                            <option key={src.id} value={src.alias}>
                              {src.name} ({src.alias})
                            </option>
                          ))}
                        </select>
                      </div>

                      <button
                        type="button"
                        onClick={() => removePathRow(index)}
                        disabled={resourcePaths.length <= 1}
                        className="p-1 rounded-control text-muted hover:text-danger hover:bg-danger-wash disabled:opacity-30"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </div>
                  ))}
                </div>
              </div>

              <div className="flex justify-end gap-3 pt-2">
                <button
                  type="button"
                  onClick={() => {
                    setIsOpen(false);
                    setEditingService(null);
                  }}
                  className="px-4 py-2 border border-rule hover:border-faint rounded-control text-ink-2 hover:bg-paper-2 transition"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={saveMutation.isPending}
                  className="px-4 py-2 bg-accent hover:bg-accent-deep text-accent-ink font-semibold rounded-control transition"
                >
                  {saveMutation.isPending ? 'Publishing...' : 'Save Service'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {isLoading ? (
        <div className="space-y-4">
          {[...Array(2)].map((_, i) => (
            <div key={i} className="h-16 bg-paper rounded-surface border border-rule animate-pulse"></div>
          ))}
        </div>
      ) : services.length === 0 ? (
        <div className="p-8 bg-paper border border-rule border-dashed rounded-surface text-center text-xs text-muted">
          No services published yet. Publish one to build routing gateways.
        </div>
      ) : (
        <div className="space-y-3">
          {services.map((svc) => (
            <div key={svc.id} className="bg-paper border border-rule p-4 rounded-surface shadow-sm hover:border-faint transition flex flex-col sm:flex-row sm:items-center justify-between gap-4">
              <div className="flex items-start gap-3">
                <div className="p-2 rounded-control bg-paper-2 text-muted mt-1">
                  <Server className="w-5 h-5" />
                </div>
                <div>
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="font-title text-sm font-semibold text-ink">{svc.name}</span>
                    <span className="text-[9px] uppercase font-bold tracking-wider px-2 py-0.5 rounded-chip bg-accent-wash border border-accent-edge text-accent">
                      {svc.type}
                    </span>
                    {svc.isPublic && (
                      <span className="text-[9px] uppercase font-bold tracking-wider px-2 py-0.5 rounded-chip bg-ok-wash border border-ok-edge text-ok">
                        Public Route
                      </span>
                    )}
                    {svc.enabled === false && (
                      <span className="text-[9px] uppercase font-bold tracking-wider px-2 py-0.5 rounded-chip bg-danger-wash border border-danger-edge text-danger">
                        Disabled
                      </span>
                    )}
                  </div>
                  <p className="text-xs text-muted mt-1">{svc.description || 'No description.'}</p>
                  <div className="mt-2 flex gap-4 text-[10px] text-muted font-mono">
                    <span>Base: {svc.basePath}</span>
                    <span>Paths: {svc.resourcePaths?.length || 0} mapped</span>
                  </div>
                </div>
              </div>

              <div className="flex items-center gap-2 self-end sm:self-center">
                <Link
                  href={`/admin/services/${svc.id}/spec`}
                  className="px-2.5 py-1 bg-paper border border-rule hover:border-faint text-[10px] font-semibold rounded-chip text-ink-2 hover:bg-paper-2 transition flex items-center gap-1"
                >
                  <BookOpen className="w-3.5 h-3.5" /> Spec Editor
                </Link>
                <button
                  onClick={() => setEditingService(svc)}
                  className="p-1.5 rounded-control text-muted hover:text-ink hover:bg-paper-3 transition"
                  title="Edit Service"
                >
                  <Edit className="w-4 h-4" />
                </button>
                <button
                  onClick={() => {
                    if (confirm(`Delete service "${svc.name}"?`)) {
                      deleteMutation.mutate(svc.id);
                    }
                  }}
                  disabled={deleteMutation.isPending}
                  className="p-1.5 rounded-control text-muted hover:text-danger hover:bg-danger-wash border border-transparent hover:border-danger-edge transition"
                  title="Delete Service"
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
