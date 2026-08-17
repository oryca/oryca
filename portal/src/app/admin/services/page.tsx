'use client';

import React, { useState, useEffect, useMemo } from 'react';
import { useQuery, useMutation, useQueryClient, type QueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import NavigationShell from '@/components/NavigationShell';
import Link from 'next/link';
import {
  PageHeader,
  SkeletonLine,
  useToast,
  useConfirm,
} from '@/components/ui';
import {
  Database,
  Server,
  Plus,
  Trash2,
  Edit,
  AlertTriangle,
  Sparkles,
  BookOpen,
  Sliders,
  Search,
  X,
  Filter,
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
  ogc?: { version?: string; parts?: string[] } | null;
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
      <PageHeader
        title="Manage Services"
        description="Upstream servers, the routes you publish from them, and the rules that rewrite responses"
      />

      <div className="space-y-6">
        <div role="tablist" aria-label="View" className="flex gap-6 border-b border-rule">
          <button
            role="tab"
            type="button"
            aria-selected={activeTab === 'services'}
            onClick={() => setActiveTab('services')}
            className={`-mb-px flex items-center gap-2 border-b-2 pb-3 text-sm whitespace-nowrap transition-colors duration-200 ${
              activeTab === 'services'
                ? 'border-accent font-medium text-ink'
                : 'border-transparent text-muted hover:text-ink'
            }`}
          >
            <Server className="h-4 w-4" aria-hidden="true" />
            Published services
          </button>
          <button
            role="tab"
            type="button"
            aria-selected={activeTab === 'sources'}
            onClick={() => setActiveTab('sources')}
            className={`-mb-px flex items-center gap-2 border-b-2 pb-3 text-sm whitespace-nowrap transition-colors duration-200 ${
              activeTab === 'sources'
                ? 'border-accent font-medium text-ink'
                : 'border-transparent text-muted hover:text-ink'
            }`}
          >
            <Database className="h-4 w-4" aria-hidden="true" />
            Upstream sources
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
  queryClient: QueryClient;
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
  const { toast, error: toastError } = useToast();
  const confirm = useConfirm();

  const [searchSourceQuery, setSearchSourceQuery] = useState('');

  const filteredSources = useMemo(() => {
    return sources.filter((src) => {
      const q = searchSourceQuery.toLowerCase().trim();
      return (
        !q ||
        src.name.toLowerCase().includes(q) ||
        src.alias.toLowerCase().includes(q) ||
        (src.url && src.url.toLowerCase().includes(q)) ||
        (src.description && src.description.toLowerCase().includes(q))
      );
    });
  }, [sources, searchSourceQuery]);

  // ปรับ state ระหว่าง render ตามแนวทางของ React ไม่ใช่ใน effect (กัน cascading render)
  const sourceFormKey = editingSource?.id ?? (isOpen ? 'new' : 'closed');
  const [syncedSourceKey, setSyncedSourceKey] = useState(sourceFormKey);
  if (sourceFormKey !== syncedSourceKey) {
    setSyncedSourceKey(sourceFormKey);
    if (editingSource) {
      setAlias(editingSource.alias || '');
      setName(editingSource.name || '');
      setDescription(editingSource.description || '');
      setType(editingSource.type || 'api');
      setProtocol(editingSource.protocol || 'https');
      setUrl(editingSource.url || '');
      setContentType(editingSource.contentType || 'application/json');
      setBody(editingSource.body || '');
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
  }

  const saveMutation = useMutation({
    mutationFn: async () => {
      setFormError(null);
      if (editingSource) {
        await api.put(`/sources/${editingSource.id}`, {
          alias,
          name,
          description,
          type,
          protocol,
          url: type === 'api' ? url : undefined,
          // ฟอร์มนี้ไม่ได้แก้ headers แต่ PUT เป็น replace ไม่ส่งคืนไปเท่ากับล้างทิ้ง
          headers: editingSource.headers ?? [],
          contentType: type === 'static' ? contentType : undefined,
          body: type === 'static' ? body : undefined,
        });
      } else {
        await api.post('/sources', {
          alias,
          name,
          description,
          type,
          protocol,
          url: type === 'api' ? url : undefined,
          contentType: type === 'static' ? contentType : undefined,
          body: type === 'static' ? body : undefined,
        });
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-sources'] });
      setIsOpen(false);
      setEditingSource(null);
      toast({ tone: 'ok', message: 'Upstream saved' });
    },
    onError: (err: unknown) => {
      toastError(err instanceof Error && err.message ? err.message : 'Could not save the upstream.');
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
      <div className="flex flex-col sm:flex-row justify-between sm:items-center gap-3">
        <div>
          <h2 className="text-xs font-bold text-muted uppercase tracking-wider">Upstream Server Sources</h2>
          <p className="text-xs text-muted mt-0.5">
            {sources.length} upstream source{sources.length === 1 ? '' : 's'} configured
          </p>
        </div>
        <div className="flex items-center gap-2">
          {sources.length > 0 && (
            <div className="relative min-w-[220px]">
              <Search className="w-3.5 h-3.5 text-muted absolute left-2.5 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                value={searchSourceQuery}
                onChange={(e) => setSearchSourceQuery(e.target.value)}
                placeholder="Search upstreams..."
                className="w-full bg-paper border border-rule rounded-control pl-8 pr-7 py-1.5 text-xs text-ink placeholder:text-muted outline-none focus:border-focus"
              />
              {searchSourceQuery && (
                <button
                  type="button"
                  onClick={() => setSearchSourceQuery('')}
                  className="absolute right-2 top-1/2 -translate-y-1/2 text-muted hover:text-ink"
                >
                  <X className="w-3 h-3" />
                </button>
              )}
            </div>
          )}
          <button
            onClick={() => {
              setEditingSource(null);
              setIsOpen(true);
            }}
            className="px-3 py-1.5 bg-accent hover:bg-accent-deep text-accent-ink text-xs font-semibold rounded-control transition duration-short flex items-center gap-1 shrink-0"
          >
            <Plus className="w-4 h-4" /> Add Source
          </button>
        </div>
      </div>

      {isOpen && (
        <div className="fixed inset-0 z-50 bg-scrim flex items-center justify-center p-4">
          <div className="bg-paper border border-rule rounded-surface p-6 w-full max-w-lg shadow-sm space-y-4 max-h-[90vh] overflow-y-auto">
            <h3 className="font-title text-base font-bold text-ink">
              {editingSource ? 'Edit Upstream Source' : 'New Upstream Source'}
            </h3>

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
                    Display Name
                  </label>
                  <input
                    type="text"
                    required
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="e.g. Sentinel-2 Imagery"
                    className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                  />
                </div>
                <div>
                  <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                    Alias (Identifier)
                  </label>
                  <input
                    type="text"
                    required
                    value={alias}
                    onChange={(e) => setAlias(e.target.value)}
                    placeholder="e.g. s2-imagery"
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
                  placeholder="Optional notes about this backend"
                  className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                />
              </div>

              <div>
                <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                  Source Type
                </label>
                <div className="flex gap-4">
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="radio"
                      name="sourceType"
                      value="api"
                      checked={type === 'api'}
                      onChange={() => setType('api')}
                      className="accent-accent"
                    />
                    Forward to a server (Live API)
                  </label>
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="radio"
                      name="sourceType"
                      value="static"
                      checked={type === 'static'}
                      onChange={() => setType('static')}
                      className="accent-accent"
                    />
                    Answer with a fixed body
                  </label>
                </div>
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
            <SkeletonLine key={i} className="h-16" />
          ))}
        </div>
      ) : sources.length === 0 ? (
        <div className="p-8 bg-paper border border-rule border-dashed rounded-surface text-center text-xs text-muted">
          No upstream sources configured. Configure one to link routing paths to.
        </div>
      ) : filteredSources.length === 0 ? (
        <div className="p-8 bg-paper border border-rule border-dashed rounded-surface text-center text-xs text-muted">
          No upstream sources match &quot;{searchSourceQuery}&quot;.
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {filteredSources.map((src) => (
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
                <p className="text-xs text-muted mt-2 line-clamp-2">{src.description || 'No description'}</p>
              </div>
              <div className="space-y-2 border-t border-rule pt-2">
                <div className="flex justify-between items-center text-[10px] font-mono text-muted">
                  <span className="truncate max-w-[200px]" title={src.url}>{src.type === 'api' ? src.url : 'Fixed Static Body'}</span>
                  <span className="capitalize">{src.type}</span>
                </div>
                <div className="flex justify-end gap-2">
                  <button
                    onClick={() => {
                      setEditingSource(src);
                      setIsOpen(true);
                    }}
                    className="p-1 text-muted hover:text-ink hover:bg-paper-2 rounded-control transition"
                    title="Edit source"
                  >
                    <Edit className="w-3.5 h-3.5" />
                  </button>
                  <button
                    onClick={async () => {
                      const wants = await confirm({
                        title: 'Delete this upstream source?',
                        description: `Any routes pointing to ${src.alias} will stop working until they are pointed somewhere else.`,
                        confirmLabel: 'Delete source',
                      });
                      if (wants) deleteMutation.mutate(src.id);
                    }}
                    className="p-1 text-muted hover:text-danger hover:bg-danger-wash rounded-control transition"
                    title="Delete source"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
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
  queryClient: QueryClient;
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

  const [searchQuery, setSearchQuery] = useState('');
  const [typeFilter, setTypeFilter] = useState('ALL');

  const serviceTypes = useMemo(() => {
    const types = Array.from(new Set(services.map((s) => s.type)));
    return types;
  }, [services]);

  const filteredServices = useMemo(() => {
    return services.filter((svc) => {
      const q = searchQuery.toLowerCase().trim();
      const matchesSearch =
        !q ||
        svc.name.toLowerCase().includes(q) ||
        svc.basePath.toLowerCase().includes(q) ||
        (svc.description && svc.description.toLowerCase().includes(q)) ||
        svc.type.toLowerCase().includes(q);

      const matchesType = typeFilter === 'ALL' || svc.type === typeFilter;
      return matchesSearch && matchesType;
    });
  }, [services, searchQuery, typeFilter]);

  const [formError, setFormError] = useState<string | null>(null);
  const { toast } = useToast();
  const confirm = useConfirm();
  const [scaffoldFetched, setScaffoldFetched] = useState<string[]>([]);
  const [isScaffoldingLoading, setIsScaffoldingLoading] = useState(false);

  // Adding an upstream from inside this form, so nobody has to leave for the other tab
  const [newSourceRow, setNewSourceRow] = useState<number | null>(null);
  const [newSourceAlias, setNewSourceAlias] = useState('');
  const [newSourceUrl, setNewSourceUrl] = useState('');
  const [newSourceKind, setNewSourceKind] = useState<'api' | 'static'>('api');
  const [newSourceBody, setNewSourceBody] = useState('');
  const [newSourceError, setNewSourceError] = useState<string | null>(null);
  const [isCreatingSource, setIsCreatingSource] = useState(false);

  const createInlineSource = async (rowIndex: number) => {
    setNewSourceError(null);
    const alias = newSourceAlias.trim();
    const url = newSourceUrl.trim();
    if (!alias) {
      setNewSourceError('The upstream needs a short name.');
      return;
    }
    if (newSourceKind === 'api' && !url) {
      setNewSourceError('Where should this path go? Give it a URL.');
      return;
    }
    if (newSourceKind === 'static' && !newSourceBody.trim()) {
      setNewSourceError('A fixed reply needs a body.');
      return;
    }
    if (sources.some((s) => s.alias === alias)) {
      setNewSourceError(`An upstream called "${alias}" already exists.`);
      return;
    }
    const withScheme = /^https?:\/\//i.test(url) ? url : `https://${url}`;
    setIsCreatingSource(true);
    try {
      await api.post('/sources', {
        alias,
        name: alias,
        type: newSourceKind,
        protocol: newSourceKind === 'static'
          ? 'https'
          : withScheme.toLowerCase().startsWith('http://') ? 'http' : 'https',
        url: newSourceKind === 'static' ? '' : withScheme,
        body: newSourceKind === 'static' ? newSourceBody : undefined,
        contentType: 'application/json',
      });
      await queryClient.invalidateQueries({ queryKey: ['admin-sources'] });
      // Map to all unmapped rows and current row
      setResourcePaths((prev) =>
        prev.map((rp, i) =>
          i === rowIndex || !rp.sourceAlias ? { ...rp, sourceAlias: alias } : rp
        )
      );
      setNewSourceRow(null);
      setNewSourceAlias('');
      setNewSourceUrl('');
      setNewSourceBody('');
      setNewSourceKind('api');
    } catch (err: unknown) {
      setNewSourceError(err instanceof Error && err.message ? err.message : 'Could not create the upstream.');
    } finally {
      setIsCreatingSource(false);
    }
  };

  // ปรับ state ระหว่าง render ตามแนวทางของ React ไม่ใช่ใน effect (กัน cascading render)
  const serviceFormKey = editingService?.id ?? (isOpen ? 'new' : 'closed');
  const [syncedServiceKey, setSyncedServiceKey] = useState(serviceFormKey);
  if (serviceFormKey !== syncedServiceKey) {
    setSyncedServiceKey(serviceFormKey);
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
    } else {
      setName('');
      setDescription('');
      setType('General');
      setBasePath('');
      setEnabled(true);
      setIsPublic(false);
      setResourcePaths([{ path: '/', methods: ['GET'], sourceAlias: '' }]);
    }
    setScaffoldFetched([]);
  }

  // ประเภท General ไม่มีเส้นทางมาตรฐานให้แนะนำ กรองตอน render แทนที่จะสั่งล้าง state ใน effect
  const scaffoldSuggestions = basePath && type !== 'General' ? scaffoldFetched : [];

  // เสนอเส้นทาง OGC ที่ยังขาด เมื่อประเภทหรือ base path เปลี่ยน
  useEffect(() => {
    if (!basePath || type === 'General') return;

    let cancelled = false;

    async function checkPaths() {
      setIsScaffoldingLoading(true);
      try {
        const pathsPayload = {
          type,
          basePath,
          resourcePaths: resourcePaths.map((r) => r.path),
        };
        const res = await api.post('/services/check-paths', pathsPayload);
        if (!cancelled && res.data.missingOgcPaths) {
          setScaffoldFetched(res.data.missingOgcPaths);
        }
      } catch {
        // เช็คเส้นทางล้มเหลวไม่ใช่เรื่องที่ผู้ใช้ต้องรู้ แค่ไม่มีคำแนะนำขึ้น
      } finally {
        if (!cancelled) setIsScaffoldingLoading(false);
      }
    }

    // หน่วงระหว่างพิมพ์ ไม่ให้ยิงทุกตัวอักษร
    const delay = setTimeout(checkPaths, 800);
    return () => {
      cancelled = true;
      clearTimeout(delay);
    };
  }, [type, basePath, resourcePaths]);

  const addPathRow = (pathValue = '/') => {
    const activeAlias =
      resourcePaths.find((r) => r.sourceAlias)?.sourceAlias || sources[0]?.alias || '';
    setResourcePaths((prev) => [
      ...prev,
      { path: pathValue, methods: ['GET'], sourceAlias: activeAlias },
    ]);
  };

  const applyUpstreamToAll = (alias: string) => {
    if (!alias || alias === '__new__') return;
    setResourcePaths((prev) => prev.map((rp) => ({ ...rp, sourceAlias: alias })));
    toast({ tone: 'ok', message: `Applied upstream "${alias}" to all routes` });
  };

  const removePathRow = (index: number) => {
    setResourcePaths((prev) => prev.filter((_, i) => i !== index));
  };

  const updatePathRow = (index: number, field: keyof ResourcePath, value: string | string[]) => {
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
        // ฟอร์มนี้ไม่ได้แก้ ogc แต่ PUT เป็น replace ไม่ส่งคืนไปเท่ากับล้างทิ้ง
        ogc: editingService?.ogc ?? undefined,
      };

      let serviceId = editingService?.id;
      if (editingService) {
        await api.put(`/services/${editingService.id}`, payload);
      } else {
        const res = await api.post('/services', payload);
        serviceId = res.data?.id;
      }

      // An OGC service almost always needs its links rewritten, so offer it here
      // rather than leaving it to be discovered on another page.
      if (serviceId && type !== 'General') {
        try {
          const presets = await api.get(`/response-transforms/presets?type=${type}`);
          const preset = (presets.data.items || [])[0];
          const wantsPreset =
            preset &&
            (await confirm({
              title: 'Rewrite the links this service returns?',
              description: `${name} answers with links to itself, which would send clients past the gateway.`,
              consequences: [`Applies the "${preset.title || preset.name}" preset`, 'You can edit or remove it later under Transforms & Presets'],
              confirmLabel: 'Apply preset',
              cancelLabel: 'Not now',
            }));
          if (wantsPreset) {
            await api.post(`/response-transforms/presets/${preset.name}/apply`, { serviceId });
          }
        } catch (err) {
          console.error('Could not offer the link-rewrite preset:', err);
        }
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-services'] });
      setIsOpen(false);
      setEditingService(null);
      toast({ tone: 'ok', message: 'Service saved' });
    },
    onError: (err: unknown) => {
      setFormError(err instanceof Error && err.message ? err.message : 'Failed to save service.');
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
      <div className="flex flex-col sm:flex-row justify-between sm:items-center gap-3">
        <div>
          <h2 className="text-xs font-bold text-muted uppercase tracking-wider">Publish Routing Gateways</h2>
          <p className="text-xs text-muted mt-0.5">
            {services.length} published gateway service{services.length === 1 ? '' : 's'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {services.length > 0 && (
            <div className="relative min-w-[220px]">
              <Search className="w-3.5 h-3.5 text-muted absolute left-2.5 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search services or paths..."
                className="w-full bg-paper border border-rule rounded-control pl-8 pr-7 py-1.5 text-xs text-ink placeholder:text-muted outline-none focus:border-focus"
              />
              {searchQuery && (
                <button
                  type="button"
                  onClick={() => setSearchQuery('')}
                  className="absolute right-2 top-1/2 -translate-y-1/2 text-muted hover:text-ink"
                >
                  <X className="w-3 h-3" />
                </button>
              )}
            </div>
          )}
          <button
            onClick={() => {
              setEditingService(null);
              setIsOpen(true);
            }}
            className="px-3 py-1.5 bg-accent hover:bg-accent-deep text-accent-ink text-xs font-semibold rounded-control transition duration-short flex items-center gap-1 shrink-0"
          >
            <Plus className="w-4 h-4" /> Publish Service
          </button>
        </div>
      </div>

      {/* Type Filter Pills */}
      {serviceTypes.length > 1 && (
        <div className="flex flex-wrap gap-1.5 items-center">
          <span className="text-[11px] text-muted flex items-center gap-1 mr-1">
            <Filter className="w-3 h-3" /> Type:
          </span>
          <button
            type="button"
            onClick={() => setTypeFilter('ALL')}
            className={`px-2.5 py-1 text-[11px] rounded-control transition-colors ${
              typeFilter === 'ALL'
                ? 'bg-accent text-accent-ink font-semibold'
                : 'bg-paper-2 border border-rule text-muted hover:text-ink'
            }`}
          >
            All ({services.length})
          </button>
          {serviceTypes.map((t) => {
            const count = services.filter((s) => s.type === t).length;
            return (
              <button
                key={t}
                type="button"
                onClick={() => setTypeFilter(t)}
                className={`px-2.5 py-1 text-[11px] rounded-control transition-colors ${
                  typeFilter === t
                    ? 'bg-accent text-accent-ink font-semibold'
                    : 'bg-paper-2 border border-rule text-muted hover:text-ink'
                }`}
              >
                {t} ({count})
              </button>
            );
          })}
        </div>
      )}

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
              {/* คำแนะนำเส้นทางมาจาก POST /services/check-paths ซึ่งบังคับต้องมี basePath
                * (มันทำหน้าที่ตรวจ path ชนกันด้วย) เลือก type อย่างเดียวจึงยังไม่มีอะไรขึ้น
                * ถ้าไม่บอก ผู้ใช้จะนึกว่าฟีเจอร์พัง */}
              {type !== 'General' && !basePath && (
                <p className="mt-2 text-xs text-muted">
                  Fill in the base path to see the routes this standard expects.
                </p>
              )}

              {isScaffoldingLoading && scaffoldSuggestions.length === 0 && (
                <p className="mt-2 text-xs text-muted">Checking the standard routes for this type…</p>
              )}

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
                  <div className="flex items-center gap-3">
                    {resourcePaths.length > 1 && resourcePaths[0]?.sourceAlias && (
                      <button
                        type="button"
                        onClick={() => applyUpstreamToAll(resourcePaths[0].sourceAlias)}
                        className="text-[11px] text-accent font-semibold hover:underline"
                        title="Set all routes to use this upstream"
                      >
                        ⚡ Apply &quot;{resourcePaths[0].sourceAlias}&quot; to all
                      </button>
                    )}
                    <button
                      type="button"
                      onClick={() => addPathRow()}
                      className="text-xs text-accent font-semibold hover:underline"
                    >
                      + Add Path Row
                    </button>
                  </div>
                </div>

                <div className="space-y-2 max-h-52 overflow-y-auto pr-1">
                  {resourcePaths.map((rp, index) => (
                    <React.Fragment key={index}>
                      <div className="flex gap-2 items-center bg-paper-2 p-2 border border-rule rounded-control">
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
                          onChange={(e) => {
                            if (e.target.value === '__new__') {
                              setNewSourceError(null);
                              setNewSourceRow(index);
                              return;
                            }
                            updatePathRow(index, 'sourceAlias', e.target.value);
                          }}
                          className="w-full bg-paper border border-rule rounded-control px-2.5 py-1 text-xs text-ink outline-none"
                        >
                          <option value="">
                            {sources.length === 0 ? 'No upstreams yet' : 'Map upstream...'}
                          </option>
                          {sources.map((src) => (
                            <option key={src.id} value={src.alias}>
                              {src.name} ({src.alias})
                            </option>
                          ))}
                          <option value="__new__">+ New upstream...</option>
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

                      {newSourceRow === index && (
                        <div className="ml-2 mb-2 p-2.5 bg-accent-wash border border-accent/30 rounded-control space-y-2">
                          <div className="flex items-center gap-3 text-[11px]">
                            <label className="flex items-center gap-1.5 cursor-pointer">
                              <input
                                type="radio"
                                checked={newSourceKind === 'api'}
                                onChange={() => setNewSourceKind('api')}
                                className="accent-accent"
                              />
                              Forward to a server
                            </label>
                            <label className="flex items-center gap-1.5 cursor-pointer">
                              <input
                                type="radio"
                                checked={newSourceKind === 'static'}
                                onChange={() => setNewSourceKind('static')}
                                className="accent-accent"
                              />
                              Answer with a fixed body
                            </label>
                          </div>

                          <p className="text-[11px] text-ink-2">
                            {newSourceKind === 'api'
                              ? 'The whole destination address, not a prefix: this path is sent exactly there.'
                              : 'Nothing is forwarded. The gateway answers with this body, for an endpoint your server does not have.'}
                          </p>
                          <div className="flex gap-2">
                            <input
                              type="text"
                              value={newSourceAlias}
                              onChange={(e) => setNewSourceAlias(e.target.value)}
                              placeholder="my-features"
                              className="flex-1 min-w-0 bg-paper border border-rule rounded-control px-2.5 py-1 text-xs text-ink outline-none focus:border-focus font-mono"
                            />
                            {newSourceKind === 'api' && (
                              <input
                                type="text"
                                value={newSourceUrl}
                                onChange={(e) => setNewSourceUrl(e.target.value)}
                                placeholder="https://demo.pygeoapi.io/master/collections"
                                className="flex-[2] min-w-0 bg-paper border border-rule rounded-control px-2.5 py-1 text-xs text-ink outline-none focus:border-focus font-mono"
                              />
                            )}
                            <button
                              type="button"
                              onClick={() => createInlineSource(index)}
                              disabled={isCreatingSource}
                              className="px-3 py-1 text-xs font-semibold rounded-control bg-accent text-accent-ink disabled:opacity-50"
                            >
                              {isCreatingSource ? 'Adding...' : 'Add'}
                            </button>
                            <button
                              type="button"
                              onClick={() => setNewSourceRow(null)}
                              className="px-2 py-1 text-xs text-muted hover:text-ink"
                            >
                              Cancel
                            </button>
                          </div>
                          {newSourceKind === 'static' && (
                            <>
                              <textarea
                                value={newSourceBody}
                                onChange={(e) => setNewSourceBody(e.target.value)}
                                rows={4}
                                placeholder={'{ "links": [] }'}
                                className="w-full bg-paper border border-rule rounded-control px-2.5 py-1.5 text-[11px] text-ink outline-none focus:border-focus font-mono"
                              />
                              {rp.path.replace(/\/$/, '').endsWith('/conformance') && (
                                <p className="text-[11px] text-warn">
                                  A conformance document states which classes a server implements.
                                  Writing one here declares that on its behalf, so list only what it
                                  really does.
                                </p>
                              )}
                            </>
                          )}

                          {newSourceError && (
                            <p className="text-[11px] text-danger">{newSourceError}</p>
                          )}
                          <p className="text-[11px] text-muted">
                            More settings, like a static body or a different content type, live under
                            the Upstream Sources tab.
                          </p>
                        </div>
                      )}
                    </React.Fragment>
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
            <SkeletonLine key={i} className="h-16" />
          ))}
        </div>
      ) : services.length === 0 ? (
        <div className="p-8 bg-paper border border-rule border-dashed rounded-surface text-center text-xs text-muted">
          No services published yet. Publish one to start routing traffic.
        </div>
      ) : filteredServices.length === 0 ? (
        <div className="p-8 bg-paper border border-rule border-dashed rounded-surface text-center text-xs text-muted">
          No services match your search &quot;{searchQuery}&quot;
          {typeFilter !== 'ALL' ? ` in type ${typeFilter}` : ''}.
        </div>
      ) : (
        <div className="space-y-3">
          {filteredServices.map((svc) => (
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
                  href={`/admin/transforms?serviceId=${svc.id}`}
                  className="px-2.5 py-1 bg-paper border border-rule hover:border-faint text-[10px] font-semibold rounded-chip text-ink-2 hover:bg-paper-2 transition flex items-center gap-1"
                >
                  <Sliders className="w-3.5 h-3.5" /> Transforms
                </Link>
                <Link
                  href={`/admin/services/${svc.id}/spec`}
                  className="px-2.5 py-1 bg-paper border border-rule hover:border-faint text-[10px] font-semibold rounded-chip text-ink-2 hover:bg-paper-2 transition flex items-center gap-1"
                >
                  <BookOpen className="w-3.5 h-3.5" /> Spec Editor
                </Link>
                <button
                  onClick={() => {
                    setEditingService(svc);
                    setIsOpen(true);
                  }}
                  className="p-1.5 rounded-control text-muted hover:text-ink hover:bg-paper-3 transition"
                  title="Edit Service"
                >
                  <Edit className="w-4 h-4" />
                </button>
                <button
                  onClick={async () => {
                    const ok = await confirm({
                      title: `Delete service "${svc.name}"`,
                      consequences: [
                        'This cannot be undone',
                        'Its routes stop answering through the gateway immediately',
                        'Its spec and rewrite rules go with it',
                      ],
                      typeToConfirm: svc.name,
                      confirmLabel: 'Delete service',
                      danger: true,
                    });
                    if (ok) deleteMutation.mutate(svc.id);
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
