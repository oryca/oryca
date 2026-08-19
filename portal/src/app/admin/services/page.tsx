'use client';

import { useState, useMemo, useRef } from 'react';
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
  BookOpen,
  Sliders,
  Search,
  X,
  Filter,
  ChevronDown,
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

/** The upstream a route carries with it, saved in the same call as the service */
interface InlineSource {
  type: 'api' | 'static';
  protocol?: string;
  url?: string;
  contentType?: string;
  body?: string;
  headers?: Array<{ key: string; value: string }>;
}

interface ResourcePath {
  path: string;
  methods: string[];
  sourceAlias: string;
  /** Present when this route defines its own upstream instead of reusing a saved one */
  source?: InlineSource;
}

interface GatewayService {
  id: string;
  name: string;
  description?: string;
  type: string; // General | OGC_API_Features | OGC_API_STAC | OGC_API_Styles | OGC_API_SensorThings | OGC_API_Tiles
  basePath: string;
  enabled?: boolean;
  isPublic?: boolean;
  resourcePaths?: Array<ResourcePath & { source?: GatewaySource }>;
  ogc?: { version?: string; parts?: string[] } | null;
}

/** Aliases the control plane generates for a route's own upstream (`src-<objectid>`).
 *  Those belong to the route, so the form edits them in place; anything else is a
 *  named source that other services may share, and is only ever referenced. */
const GENERATED_ALIAS = /^src-[0-9a-f]{24}$/i;

/** A saved route becomes an editable row, with the upstream it resolved to opened
 *  up for editing in place — one route, one target. */
function toFormRow(rp: ResourcePath & { source?: GatewaySource }): ResourcePath {
  const base = { path: rp.path, methods: rp.methods, sourceAlias: rp.sourceAlias };
  if (!rp.source) return base;
  // the API hands back a source without its url or body when the caller is not
  // allowed to see them; editing that as if it were empty would erase it
  const isStatic = rp.source.type === 'static';
  if (isStatic ? !rp.source.body : !rp.source.url) return base;
  return {
    ...base,
    source: {
      type: rp.source.type === 'static' ? 'static' : 'api',
      protocol: rp.source.protocol,
      url: rp.source.url || '',
      contentType: rp.source.contentType || 'application/json',
      body: rp.source.body || '',
      // PUT replaces the whole source, so headers set elsewhere ride along untouched
      headers: rp.source.headers || [],
    },
  };
}

/** A row as the service payload wants it. An inline target rides along under
 *  `source`; an empty alias tells the control plane to mint one for it. */
function toApiRow(rp: ResourcePath): ResourcePath {
  const row = { path: rp.path, methods: rp.methods, sourceAlias: rp.sourceAlias };
  if (!rp.source) return row;
  if (rp.source.type === 'static') {
    return {
      ...row,
      source: {
        ...rp.source,
        protocol: 'https',
        url: '',
        contentType: (rp.source.contentType || '').trim() || 'application/json',
      },
    };
  }
  const url = (rp.source.url || '').trim();
  const withScheme = /^https?:\/\//i.test(url) ? url : `https://${url}`;
  return {
    ...row,
    source: {
      ...rp.source,
      url: withScheme,
      protocol: withScheme.toLowerCase().startsWith('http://') ? 'http' : 'https',
      body: '',
    },
  };
}

function targetSelectValue(rp: ResourcePath): string {
  if (!rp.source) return '';
  return rp.source.type === 'static' ? '__static__' : '__api__';
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
            <div className="flex items-center justify-between">
              <h3 className="font-title text-base font-bold text-ink">
                {editingSource ? 'Edit Upstream Source' : 'New Upstream Source'}
              </h3>
              <button
                onClick={() => setIsOpen(false)}
                className="p-1.5 rounded-control text-muted transition hover:bg-paper-3 hover:text-ink cursor-pointer"
                aria-label="Close"
                title="Close"
              >
                <X className="h-4 w-4" />
              </button>
            </div>

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
                    Display Name<span className="ui-field__req" aria-hidden="true">*</span>
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
                    Alias (Identifier)<span className="ui-field__req" aria-hidden="true">*</span>
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
                    Target Base URL<span className="ui-field__req" aria-hidden="true">*</span>
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
                      Content Type<span className="ui-field__req" aria-hidden="true">*</span>
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
                  className="px-4 py-2 border border-rule hover:border-faint rounded-control text-ink-2 hover:bg-paper-2 transition cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={saveMutation.isPending}
                  className="px-4 py-2 bg-accent hover:bg-accent-deep text-accent-ink font-semibold rounded-control transition cursor-pointer"
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
                    <span className="font-title text-sm font-semibold text-ink truncate max-w-[180px]" title={src.name}>
                      {src.name}
                    </span>
                  </div>
                  <span className="text-[10px] font-mono px-2 py-0.5 rounded-chip bg-accent-wash border border-accent-edge text-accent">
                    Alias: {src.alias}
                  </span>
                </div>
                <p className="text-xs text-muted mt-2 line-clamp-2">
                  {GENERATED_ALIAS.test(src.alias)
                    ? src.description || 'Defined on a route, in the service that uses it'
                    : src.description || 'No description'}
                </p>
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
  isLoading,
  isOpen,
  setIsOpen,
  editingService,
  setEditingService,
  queryClient
}: {
  services: GatewayService[];
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
  const [isApplyingOgcTemplate, setIsApplyingOgcTemplate] = useState(false);

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
          ? editingService.resourcePaths.map(toFormRow)
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
  }

  // A path that cannot match any real standard path, sent as the current
  // resourcePaths so every core path of the type comes back as "missing" —
  // that missing list is the full template. Reuses the same backend data
  // /services/check-paths already serves, just asked for differently.
  const OGC_TEMPLATE_PROBE_PATH = '/__ogc-template-probe__';
  // check-paths requires a non-empty basePath to also check for conflicts;
  // there is no real basePath yet at the point a type is picked, and the
  // conflict result is discarded here, so a placeholder is enough.
  const OGC_TEMPLATE_PROBE_BASE_PATH = '/__ogc-template-probe__';

  async function fetchOgcCorePaths(svcType: string): Promise<string[]> {
    const res = await api.post('/services/check-paths', {
      type: svcType,
      basePath: OGC_TEMPLATE_PROBE_BASE_PATH,
      resourcePaths: [OGC_TEMPLATE_PROBE_PATH],
    });
    return res.data.missingOgcPaths || [];
  }

  /** Counts type changes so a slow template fetch cannot land after a newer one */
  const ogcTemplateRequest = useRef(0);

  /** Path work a retype would destroy: a saved service's routes, or rows already
   *  mapped to an upstream. A fresh template has neither, so it stays quiet. */
  function hasPathWork(): boolean {
    if (resourcePaths.length === 0) return false;
    return !!editingService || resourcePaths.some((rp) => rp.sourceAlias !== '' || !!rp.source);
  }

  /** Picking a standard replaces the path list outright with what it expects —
   *  there is no partial state to preserve once the type itself has changed. */
  async function applyServiceType(newType: string) {
    if (newType === type) return;
    if (hasPathWork()) {
      const wants = await confirm({
        title: 'Replace the routes below?',
        description: `Switching to ${newType === 'General' ? 'General API' : newType} rewrites the path list from scratch.`,
        consequences: [
          `The ${resourcePaths.length} route${resourcePaths.length === 1 ? '' : 's'} configured here are discarded`,
          'Their methods and upstream mappings go with them',
        ],
        confirmLabel: 'Replace routes',
        cancelLabel: 'Keep current type',
      });
      if (!wants) return;
    }

    const request = ++ogcTemplateRequest.current;
    setType(newType);
    if (newType === 'General') {
      setResourcePaths([{ path: '/', methods: ['GET'], sourceAlias: '' }]);
      return;
    }
    setIsApplyingOgcTemplate(true);
    try {
      const paths = await fetchOgcCorePaths(newType);
      if (request !== ogcTemplateRequest.current) return;
      // left unmapped on purpose — the standard only supplies the routes,
      // not which upstream answers each one, so that stays a manual choice
      setResourcePaths(
        (paths.length > 0 ? paths : ['/']).map((path) => ({
          path,
          methods: ['GET'],
          sourceAlias: '',
        })),
      );
    } catch {
      // could not reach the standard's path list — leave the rows as they were
    } finally {
      if (request === ogcTemplateRequest.current) setIsApplyingOgcTemplate(false);
    }
  }

  const addPathRow = (pathValue = '/') => {
    setResourcePaths((prev) => [...prev, { path: pathValue, methods: ['GET'], sourceAlias: '' }]);
  };

  /** Switches what a route answers with. The alias rides along untouched: an empty one
   *  has the control plane mint an upstream, an existing one is rewritten in place. */
  const setRowTarget = (index: number, value: string) => {
    setResourcePaths((prev) =>
      prev.map((rp, i) => {
        if (i !== index) return rp;
        if (value !== '__api__' && value !== '__static__') return { ...rp, source: undefined };
        const type = value === '__api__' ? 'api' : 'static';
        if (rp.source) return { ...rp, source: { ...rp.source, type } };
        return {
          ...rp,
          source: { type, protocol: 'https', url: '', contentType: 'application/json', body: '' },
        };
      }),
    );
  };

  const updateRowSource = (index: number, patch: Partial<InlineSource>) => {
    setResourcePaths((prev) =>
      prev.map((rp, i) => (i === index && rp.source ? { ...rp, source: { ...rp.source, ...patch } } : rp)),
    );
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
      resourcePaths.forEach((rp) => {
        if (rp.source) {
          if (rp.source.type === 'api' && !(rp.source.url || '').trim()) {
            throw new Error(`${rp.path} needs a URL to forward to.`);
          }
          if (rp.source.type === 'static' && !(rp.source.body || '').trim()) {
            throw new Error(`${rp.path} needs a body to answer with.`);
          }
        } else if (!rp.sourceAlias) {
          throw new Error(`${rp.path} has no target yet.`);
        }
      });
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
        resourcePaths: resourcePaths.map(toApiRow),
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
      // saving a service now creates, updates, and retires upstreams too
      queryClient.invalidateQueries({ queryKey: ['admin-sources'] });
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
            <div className="flex items-center justify-between">
              <h3 className="font-title text-base font-bold text-ink">
                {editingService ? 'Edit Gateway Service' : 'Publish New Service'}
              </h3>
              <button
                onClick={() => setIsOpen(false)}
                className="p-1.5 rounded-control text-muted transition hover:bg-paper-3 hover:text-ink cursor-pointer"
                aria-label="Close"
                title="Close"
              >
                <X className="h-4 w-4" />
              </button>
            </div>

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
                    Service Name<span className="ui-field__req" aria-hidden="true">*</span>
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
                    Base Path (Prefix route)<span className="ui-field__req" aria-hidden="true">*</span>
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
                    Service Type<span className="ui-field__req" aria-hidden="true">*</span>
                  </label>
                  <div className="relative">
                    <select
                      required
                      value={type}
                      onChange={(e) => {
                        // hold the select on the current type until confirmed —
                        // setType moves it if applyServiceType gets that far
                        const picked = e.target.value;
                        e.target.value = type;
                        applyServiceType(picked);
                      }}
                      className="w-full appearance-none bg-paper-2 border border-rule rounded-control pl-3 pr-9 py-2 text-sm text-ink outline-none focus:border-focus"
                    >
                      <option value="General">General API</option>
                      <option value="OGC_API_Features">OGC API Features</option>
                      <option value="OGC_API_STAC">OGC API STAC</option>
                      <option value="OGC_API_Styles">OGC API Styles</option>
                      <option value="OGC_API_SensorThings">OGC API SensorThings</option>
                      <option value="OGC_API_Tiles">OGC API Tiles</option>
                    </select>
                    <ChevronDown className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted" />
                  </div>
                </div>

                <div className="flex items-center gap-2 pt-5">
                  <input
                    type="checkbox"
                    id="svc-enabled"
                    checked={enabled}
                    onChange={(e) => setEnabled(e.target.checked)}
                    className="w-4 h-4 accent-accent cursor-pointer"
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
                    className="w-4 h-4 accent-accent cursor-pointer"
                  />
                  <label htmlFor="svc-public" className="text-xs font-semibold text-ink-2 select-none cursor-pointer">
                    Public access (no API key)
                  </label>
                </div>
              </div>

              {/* Picking a standard already filled the paths below in with its template */}
              {isApplyingOgcTemplate && (
                <p className="mt-2 text-xs text-muted">Loading the standard routes for this type…</p>
              )}

              {/* Resource paths editor mapping */}
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <label className="text-xs font-semibold text-muted uppercase tracking-wider">
                    Resource Paths & Target Sources<span className="ui-field__req" aria-hidden="true">*</span>
                  </label>
                  <button
                    type="button"
                    onClick={() => addPathRow()}
                    className="text-xs text-accent font-semibold underline underline-offset-2 cursor-pointer"
                  >
                    + Add Path
                  </button>
                </div>

                <div className="space-y-2 max-h-52 overflow-y-auto pr-1">
                  {resourcePaths.map((rp, index) => (
                    <div
                      key={index}
                      className="bg-paper-2 p-2 border border-rule rounded-control space-y-2"
                    >
                      <div className="flex gap-2 items-center">
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

                        <div className="relative flex-1">
                          <select
                            value={rp.methods[0] || 'GET'}
                            onChange={(e) => updatePathRow(index, 'methods', [e.target.value])}
                            className="w-full appearance-none bg-paper border border-rule rounded-control pl-2.5 pr-7 py-1 text-xs text-ink outline-none"
                          >
                            <option value="GET">GET</option>
                            <option value="POST">POST</option>
                            <option value="PUT">PUT</option>
                            <option value="DELETE">DELETE</option>
                          </select>
                          <ChevronDown className="pointer-events-none absolute right-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted" />
                        </div>

                        <div className="relative flex-[2]">
                          <select
                            required
                            value={targetSelectValue(rp)}
                            onChange={(e) => setRowTarget(index, e.target.value)}
                            className="w-full appearance-none bg-paper border border-rule rounded-control pl-2.5 pr-7 py-1 text-xs text-ink outline-none"
                          >
                            <option value="" disabled>
                              Map upstream...
                            </option>
                            <option value="__api__">Forward to a URL</option>
                            <option value="__static__">Answer with a fixed body</option>
                          </select>
                          <ChevronDown className="pointer-events-none absolute right-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted" />
                        </div>

                        <button
                          type="button"
                          onClick={() => removePathRow(index)}
                          disabled={resourcePaths.length <= 1}
                          className="p-1 rounded-control text-muted hover:text-danger hover:bg-danger-wash disabled:opacity-30 cursor-pointer"
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      </div>

                      {rp.source && (
                        <div className="space-y-2 border-t border-rule pt-2">
                          {rp.source.type === 'api' ? (
                            <>
                              <p className="text-[11px] text-ink-2">
                                The whole destination address, not a prefix: this path is sent exactly there.
                              </p>
                              <input
                                type="text"
                                required
                                value={rp.source.url || ''}
                                onChange={(e) => updateRowSource(index, { url: e.target.value })}
                                placeholder="https://demo.pygeoapi.io/master/collections"
                                className="w-full bg-paper border border-rule rounded-control px-2.5 py-1 text-xs text-ink outline-none focus:border-focus font-mono"
                              />
                            </>
                          ) : (
                            <>
                              <p className="text-[11px] text-ink-2">
                                Nothing is forwarded. The gateway answers with this body, for an
                                endpoint your server does not have.
                              </p>
                              <input
                                type="text"
                                value={rp.source.contentType || ''}
                                onChange={(e) => updateRowSource(index, { contentType: e.target.value })}
                                placeholder="application/json"
                                className="w-full bg-paper border border-rule rounded-control px-2.5 py-1 text-xs text-ink outline-none focus:border-focus font-mono"
                              />
                              <textarea
                                required
                                rows={4}
                                value={rp.source.body || ''}
                                onChange={(e) => updateRowSource(index, { body: e.target.value })}
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

                        
                        </div>
                      )}
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
                  className="px-4 py-2 border border-rule hover:border-faint rounded-control text-ink-2 hover:bg-paper-2 transition cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={saveMutation.isPending}
                  className="px-4 py-2 bg-accent hover:bg-accent-deep text-accent-ink font-semibold rounded-control transition cursor-pointer"
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
