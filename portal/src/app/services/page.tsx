'use client';

import React, { useState, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, GATEWAY_URL } from '@/lib/api';
import { useAuth } from '@/app/providers';
import NavigationShell from '@/components/NavigationShell';
import axios from 'axios';
import {
  Key,
  Server,
  BookOpen,
  Play,
  Terminal,
  Copy,
  CheckCircle,
  AlertTriangle,
  RefreshCw,
  Plus,
  Trash2,
  Calendar,
  Layers,
  ChevronRight,
  ChevronDown,
  Sparkles,
  Info
} from 'lucide-react';

interface GatewayService {
  id: string;
  name: string;
  description?: string;
  type: string;
  basePath: string;
  enabled?: boolean;
  isPublic?: boolean;
  resourcePaths?: Array<{
    path: string;
    methods: string[];
    sourceAlias: string;
  }>;
}

interface ApiKey {
  id: string;
  name: string;
  apiKey?: string;
  expiredAt?: string;
  createdAt?: string;
  enabled?: boolean;
}

interface ApiDoc {
  id: string;
  info: {
    title: string;
    version: string;
    description?: string;
  };
}

export default function ServicesAndKeysPage() {
  const queryClient = useQueryClient();
  const { user } = useAuth();
  
  const [activeTab, setActiveTab] = useState<'catalog' | 'keys'>('catalog');
  const [selectedService, setSelectedService] = useState<GatewayService | null>(null);
  
  // Details tab state
  const [serviceDetailsTab, setServiceDetailsTab] = useState<'info' | 'spec' | 'try' | 'docs'>('info');

  // API Key creation / regeneration states
  const [newKeyName, setNewKeyName] = useState('');
  const [newKeyExpiry, setNewKeyExpiry] = useState('');
  const [createdKey, setCreatedKey] = useState<string | null>(null);
  const [keySuccess, setKeySuccess] = useState<string | null>(null);
  const [keyError, setKeyError] = useState<string | null>(null);
  
  // Try It state
  const [selectedApiKey, setSelectedApiKey] = useState<string>('');
  const [selectedPath, setSelectedPath] = useState<string>('');
  const [selectedMethod, setSelectedMethod] = useState<string>('GET');
  const [customParams, setCustomParams] = useState<string>('');
  const [responseStatus, setResponseStatus] = useState<number | null>(null);
  const [responseHeaders, setResponseHeaders] = useState<Record<string, string>>({});
  const [responseBody, setResponseBody] = useState<string>('');
  const [isSendingRequest, setIsSendingRequest] = useState(false);
  const [copiedCurl, setCopiedCurl] = useState(false);

  // Queries
  const { data: services, isLoading: isLoadingServices } = useQuery<GatewayService[]>({
    queryKey: ['catalog-services'],
    queryFn: async () => {
      const res = await api.get('/services');
      return res.data.items || [];
    },
  });

  const { data: apiKeys, isLoading: isLoadingKeys } = useQuery<ApiKey[]>({
    queryKey: ['api-keys'],
    queryFn: async () => {
      const res = await api.get('/api-keys');
      return res.data.items || [];
    },
  });

  // Fetch OpenAPI Spec for selected service
  const { data: openApiSpec, isLoading: isLoadingSpec, error: specError } = useQuery<any>({
    queryKey: ['service-openapi', selectedService?.id],
    queryFn: async () => {
      if (!selectedService) return null;
      const res = await api.get(`/services/${selectedService.id}/spec/openapi`);
      return res.data;
    },
    enabled: !!selectedService && serviceDetailsTab === 'spec',
    retry: false,
  });

  // Fetch Api Doc notes
  const { data: apiDocs } = useQuery<any[]>({
    queryKey: ['api-docs'],
    queryFn: async () => {
      const res = await api.get('/api-docs');
      return res.data.items || [];
    },
    enabled: !!selectedService && serviceDetailsTab === 'docs',
  });

  // Mutations
  const createKeyMutation = useMutation({
    mutationFn: async () => {
      setKeyError(null);
      setCreatedKey(null);
      const res = await api.post('/api-keys', {
        name: newKeyName,
        expiredAt: newKeyExpiry ? new Date(newKeyExpiry).toISOString() : undefined,
      });
      return res.data;
    },
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['api-keys'] });
      setCreatedKey(data.apiKey);
      setKeySuccess('API Key created successfully. Copy it now, it will not be shown again!');
      setNewKeyName('');
      setNewKeyExpiry('');
    },
    onError: (err: any) => {
      setKeyError(err.message || 'Failed to create API key.');
    },
  });

  const deleteKeyMutation = useMutation({
    mutationFn: async (id: string) => {
      await api.delete(`/api-keys/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['api-keys'] });
    },
  });

  const regenerateKeyMutation = useMutation({
    mutationFn: async (id: string) => {
      setKeyError(null);
      setCreatedKey(null);
      const res = await api.post(`/api-keys/${id}/regenerate`);
      return res.data;
    },
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['api-keys'] });
      setCreatedKey(data.apiKey);
      setKeySuccess('API Key regenerated successfully. Copy it now, it will not be shown again!');
    },
    onError: (err: any) => {
      setKeyError(err.message || 'Failed to regenerate API key.');
    },
  });

  // Select default path for Try It when service or tab changes
  useEffect(() => {
    if (selectedService?.resourcePaths && selectedService.resourcePaths.length > 0) {
      setSelectedPath(selectedService.resourcePaths[0].path);
      setSelectedMethod(selectedService.resourcePaths[0].methods[0] || 'GET');
    } else {
      setSelectedPath('');
      setSelectedMethod('GET');
    }
    setResponseStatus(null);
    setResponseHeaders({});
    setResponseBody('');
  }, [selectedService, serviceDetailsTab]);

  // Set default API key for Try It
  useEffect(() => {
    if (apiKeys && apiKeys.length > 0 && !selectedApiKey) {
      setSelectedApiKey(apiKeys[0].apiKey || '');
    }
  }, [apiKeys, selectedApiKey]);

  // Try It Execution
  const handleTryIt = async () => {
    if (!selectedService) return;
    setIsSendingRequest(true);
    setResponseStatus(null);
    setResponseBody('');
    setResponseHeaders({});

    // Clean basePath and path prefixing
    const basePath = selectedService.basePath.replace(/^\//, '');
    const cleanPath = selectedPath.replace(/^\//, '');
    
    // Add query params if any
    let url = `${GATEWAY_URL}/resources/${basePath}/${cleanPath}`;
    if (customParams) {
      url += `?${customParams}`;
    }

    try {
      const res = await axios({
        method: selectedMethod,
        url,
        headers: selectedApiKey ? { 'X-API-Key': selectedApiKey } : {},
        timeout: 8000,
      });
      setResponseStatus(res.status);
      setResponseBody(JSON.stringify(res.data, null, 2));
      setResponseHeaders(res.headers as any);
    } catch (err: any) {
      console.error(err);
      if (err.response) {
        setResponseStatus(err.response.status);
        setResponseBody(JSON.stringify(err.response.data, null, 2));
        setResponseHeaders(err.response.headers as any);
      } else {
        setResponseStatus(500);
        setResponseBody(err.message || 'Failed to make API request.');
      }
    } finally {
      setIsSendingRequest(false);
    }
  };

  // Generate curl snippet
  const getCurlCommand = () => {
    if (!selectedService) return '';
    const basePath = selectedService.basePath.replace(/^\//, '');
    const cleanPath = selectedPath.replace(/^\//, '');
    let url = `${GATEWAY_URL}/resources/${basePath}/${cleanPath}`;
    if (customParams) {
      url += `?${customParams}`;
    }
    return `curl -X ${selectedMethod} "${url}" \\\n  -H "X-API-Key: ${selectedApiKey || '<your_key>'}"`;
  };

  const copyToClipboard = (text: string, setter: (val: boolean) => void) => {
    navigator.clipboard.writeText(text);
    setter(true);
    setTimeout(() => setter(false), 2000);
  };

  return (
    <NavigationShell>
      <div className="space-y-6">
        {/* Toggle tabs */}
        <div className="flex border-b border-rule pb-px gap-6">
          <button
            onClick={() => setActiveTab('catalog')}
            className={`pb-3 text-sm font-semibold border-b-2 transition ${
              activeTab === 'catalog'
                ? 'border-accent text-ink font-bold'
                : 'border-transparent text-muted hover:text-ink'
            }`}
          >
            📡 Service Catalog
          </button>
          <button
            onClick={() => setActiveTab('keys')}
            className={`pb-3 text-sm font-semibold border-b-2 transition ${
              activeTab === 'keys'
                ? 'border-accent text-ink font-bold'
                : 'border-transparent text-muted hover:text-ink'
            }`}
          >
            🔑 My API Keys
          </button>
        </div>

        {/* ================================= SERVICE CATALOG TAB ================================= */}
        {activeTab === 'catalog' && (
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
            {/* Catalog list */}
            <div className="lg:col-span-1 space-y-4">
              <h2 className="font-title text-base font-semibold text-ink">Published APIs</h2>
              {isLoadingServices ? (
                <div className="space-y-3">
                  {[...Array(3)].map((_, i) => (
                    <div key={i} className="h-16 bg-paper rounded-control border border-rule animate-pulse"></div>
                  ))}
                </div>
              ) : !services || services.length === 0 ? (
                <div className="p-6 bg-paper border border-rule rounded-surface text-center text-xs text-muted">
                  No services are published yet.
                </div>
              ) : (
                <div className="space-y-2">
                  {services.map((svc) => (
                    <button
                      key={svc.id}
                      onClick={() => setSelectedService(svc)}
                      className={`w-full text-left p-4 rounded-surface border transition flex items-center justify-between ${
                        selectedService?.id === svc.id
                          ? 'bg-accent-wash border-accent-edge text-ink shadow-sm'
                          : 'bg-paper border-rule hover:border-faint text-ink-2'
                      }`}
                    >
                      <div className="flex items-start gap-3">
                        <div className={`p-2 rounded-control mt-0.5 ${
                          selectedService?.id === svc.id ? 'bg-accent text-accent-ink' : 'bg-paper-2 text-muted'
                        }`}>
                          <Server className="w-4 h-4" />
                        </div>
                        <div>
                          <p className="text-sm font-semibold">{svc.name}</p>
                          <p className="text-xs text-muted font-mono">{svc.basePath}</p>
                        </div>
                      </div>
                      <ChevronRight className="w-4 h-4 text-muted" />
                    </button>
                  ))}
                </div>
              )}
            </div>

            {/* Selected service details panel */}
            <div className="lg:col-span-2">
              {!selectedService ? (
                <div className="h-64 bg-paper border border-rule border-dashed rounded-surface flex flex-col items-center justify-center text-muted p-8 text-center gap-2 shadow-sm">
                  <Server className="w-8 h-8 text-muted opacity-50" />
                  <p className="text-xs">Select an API from the list to view specifications, usage notes, and test endpoints.</p>
                </div>
              ) : (
                <div className="bg-paper border border-rule rounded-surface shadow-sm overflow-hidden flex flex-col min-h-[500px]">
                  {/* Service header */}
                  <div className="p-6 bg-paper-2 border-b border-rule">
                    <div className="flex items-start justify-between gap-4">
                      <div>
                        <span className="text-[9px] uppercase font-bold tracking-wider px-2 py-0.5 rounded-chip bg-accent-wash border border-accent-edge text-accent">
                          {selectedService.type}
                        </span>
                        <h2 className="font-title text-lg font-bold text-ink mt-1">
                          {selectedService.name}
                        </h2>
                        <p className="text-xs text-muted mt-1">{selectedService.description || 'No description provided.'}</p>
                      </div>
                    </div>
                  </div>

                  {/* Tabs inside panel */}
                  <div className="flex border-b border-rule bg-paper-2 px-6 gap-6 text-xs">
                    {(['info', 'spec', 'try', 'docs'] as const).map((t) => (
                      <button
                        key={t}
                        onClick={() => setServiceDetailsTab(t)}
                        className={`py-3 font-semibold border-b-2 uppercase tracking-wider transition ${
                          serviceDetailsTab === t
                            ? 'border-accent text-accent font-bold'
                            : 'border-transparent text-muted hover:text-ink'
                        }`}
                      >
                        {t === 'info' && 'Info'}
                        {t === 'spec' && 'OpenAPI Spec'}
                        {t === 'try' && 'Try Call'}
                        {t === 'docs' && 'Usage Docs'}
                      </button>
                    ))}
                  </div>

                  {/* Panel body */}
                  <div className="flex-1 p-6">
                    {/* TAB: INFO */}
                    {serviceDetailsTab === 'info' && (
                      <div className="space-y-4">
                        <div>
                          <h3 className="text-xs font-semibold text-muted uppercase tracking-wider mb-2">Resource Paths</h3>
                          {!selectedService.resourcePaths || selectedService.resourcePaths.length === 0 ? (
                            <p className="text-xs text-muted">No resource paths mapped.</p>
                          ) : (
                            <div className="divide-y divide-rule border border-rule rounded-surface overflow-hidden">
                              {selectedService.resourcePaths.map((rp, idx) => (
                                <div key={idx} className="p-3 flex items-center justify-between hover:bg-paper-2 transition font-mono text-xs">
                                  <div className="flex items-center gap-2">
                                    {rp.methods.map((m) => (
                                      <span key={m} className="px-1.5 py-0.5 rounded-chip bg-accent text-accent-ink font-bold text-[9px]">
                                        {m}
                                      </span>
                                    ))}
                                    <span className="text-ink font-semibold">{rp.path}</span>
                                  </div>
                                  <span className="text-muted text-[10px]">Alias: {rp.sourceAlias}</span>
                                </div>
                              ))}
                            </div>
                          )}
                        </div>

                        <div className="bg-paper-2 p-4 rounded-control border border-rule text-xs space-y-2">
                          <p className="font-semibold text-ink">Usage instructions:</p>
                          <p className="text-muted">
                            To reach this service through the gateway, call:
                          </p>
                          <pre className="p-3 bg-graphite text-graphite-ink rounded-control font-mono text-[11px] overflow-x-auto">
                            GET {GATEWAY_URL}/resources/{selectedService.basePath.replace(/^\//, '')}/&lt;path&gt;
                          </pre>
                          <p className="text-muted">
                            Remember to supply your active key in the <code className="bg-paper-3 px-1 py-0.5 rounded">X-API-Key</code> header.
                          </p>
                        </div>
                      </div>
                    )}

                    {/* TAB: OPENAPI SPEC */}
                    {serviceDetailsTab === 'spec' && (
                      <div className="space-y-4">
                        {isLoadingSpec ? (
                          <div className="flex justify-center p-8">
                            <div className="w-6 h-6 border-2 border-rule border-t-accent rounded-full animate-spin"></div>
                          </div>
                        ) : specError || !openApiSpec ? (
                          <div className="rounded-control border border-rule p-6 text-center text-xs text-muted flex flex-col items-center gap-2">
                            <Info className="w-5 h-5 text-muted" />
                            <p>No OpenAPI manual published for this service yet.</p>
                          </div>
                        ) : (
                          <div className="space-y-4">
                            <div className="flex items-center justify-between border-b border-rule pb-2">
                              <span className="text-xs font-semibold text-ink">{openApiSpec.info?.title || 'OpenAPI Specification'}</span>
                              <span className="text-xs text-muted font-mono">v{openApiSpec.info?.version || '1.0.0'}</span>
                            </div>
                            
                            {/* Render OpenAPI paths list */}
                            <div className="space-y-2 max-h-[360px] overflow-y-auto pr-1">
                              {Object.entries(openApiSpec.paths || {}).map(([path, methods]: [string, any]) => (
                                <div key={path} className="border border-rule rounded-surface p-3 bg-paper-2">
                                  {Object.entries(methods).map(([method, details]: [string, any]) => (
                                    <div key={method} className="space-y-2">
                                      <div className="flex items-center gap-2 font-mono text-xs">
                                        <span className="px-2 py-0.5 bg-accent text-accent-ink font-bold rounded-chip text-[9px] uppercase">
                                          {method}
                                        </span>
                                        <span className="font-semibold text-ink">{path}</span>
                                        {details.summary && (
                                          <span className="text-muted text-[10px] truncate max-w-xs">— {details.summary}</span>
                                        )}
                                      </div>
                                      {details.description && (
                                        <p className="text-[11px] text-muted pl-1">{details.description}</p>
                                      )}
                                    </div>
                                  ))}
                                </div>
                              ))}
                            </div>
                          </div>
                        )}
                      </div>
                    )}

                    {/* TAB: TRY IT */}
                    {serviceDetailsTab === 'try' && (
                      <div className="space-y-4 text-xs">
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                          <div>
                            <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                              Resource Path to call
                            </label>
                            <div className="relative">
                              <select
                                value={selectedPath}
                                onChange={(e) => {
                                  setSelectedPath(e.target.value);
                                  // Auto set matching method
                                  const matchingPath = selectedService.resourcePaths?.find(r => r.path === e.target.value);
                                  if (matchingPath && matchingPath.methods[0]) {
                                    setSelectedMethod(matchingPath.methods[0]);
                                  }
                                }}
                                className="w-full bg-paper-2 border border-rule rounded-control px-3 py-1.5 text-xs text-ink outline-none appearance-none focus:border-focus"
                              >
                                {selectedService.resourcePaths?.map((rp, i) => (
                                  <option key={i} value={rp.path}>{rp.path}</option>
                                ))}
                              </select>
                              <ChevronDown className="w-3.5 h-3.5 text-muted absolute right-3 top-2.5 pointer-events-none" />
                            </div>
                          </div>

                          <div>
                            <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                              API Key
                            </label>
                            <div className="relative">
                              <select
                                value={selectedApiKey}
                                onChange={(e) => setSelectedApiKey(e.target.value)}
                                className="w-full bg-paper-2 border border-rule rounded-control px-3 py-1.5 text-xs text-ink outline-none appearance-none focus:border-focus font-mono"
                              >
                                {apiKeys && apiKeys.length > 0 ? (
                                  apiKeys.map((k) => (
                                    <option key={k.id} value={k.apiKey}>{k.name} ({k.apiKey?.substring(0, 10)}...)</option>
                                  ))
                                ) : (
                                  <option value="">No API keys available</option>
                                )}
                              </select>
                              <ChevronDown className="w-3.5 h-3.5 text-muted absolute right-3 top-2.5 pointer-events-none" />
                            </div>
                          </div>
                        </div>

                        <div>
                          <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                            Query Parameters (e.g. limit=10&offset=0)
                          </label>
                          <input
                            type="text"
                            value={customParams}
                            onChange={(e) => setCustomParams(e.target.value)}
                            placeholder="f=json"
                            className="w-full bg-paper-2 border border-rule rounded-control px-3 py-1.5 text-xs text-ink outline-none focus:border-focus font-mono"
                          />
                        </div>

                        {/* cURL Display */}
                        <div className="relative group">
                          <pre className="p-3 bg-graphite text-graphite-ink rounded-control font-mono text-[10px] overflow-x-auto pr-10">
                            {getCurlCommand()}
                          </pre>
                          <button
                            onClick={() => copyToClipboard(getCurlCommand(), setCopiedCurl)}
                            className="absolute right-2 top-2 p-1.5 bg-graphite-2 hover:bg-graphite-rule text-graphite-ink rounded-control border border-graphite-rule transition opacity-70 group-hover:opacity-100"
                            title="Copy cURL"
                          >
                            {copiedCurl ? <CheckCircle className="w-3.5 h-3.5 text-ok" /> : <Copy className="w-3.5 h-3.5" />}
                          </button>
                        </div>

                        {/* Send Button */}
                        <div className="flex justify-end pt-2">
                          <button
                            onClick={handleTryIt}
                            disabled={isSendingRequest || !selectedPath}
                            className="px-4 py-2 bg-accent hover:bg-accent-deep text-accent-ink font-semibold rounded-control transition duration-short disabled:opacity-50 flex items-center gap-2"
                          >
                            <Play className="w-3.5 h-3.5 fill-current" />
                            {isSendingRequest ? 'Calling Gateway...' : 'Send Request'}
                          </button>
                        </div>

                        {/* Diagnostic result panel */}
                        {(responseStatus !== null || responseBody) && (
                          <div className="border border-rule rounded-surface overflow-hidden divide-y divide-rule">
                            {/* Headers & Status */}
                            <div className="p-3 bg-paper-2 flex flex-wrap items-center justify-between gap-4 font-mono text-[10px]">
                              <div className="flex items-center gap-2">
                                Status:
                                <span className={`px-2 py-0.5 rounded-chip font-bold border ${
                                  responseStatus && responseStatus < 400
                                    ? 'text-ok bg-ok-wash border-ok-edge'
                                    : 'text-danger bg-danger-wash border-danger-edge'
                                }`}>
                                  {responseStatus}
                                </span>
                              </div>

                              {/* Rate limit headers */}
                              <div className="flex flex-wrap gap-x-3 text-muted">
                                {responseHeaders['ratelimit-remaining'] && (
                                  <span>Remaining: {responseHeaders['ratelimit-remaining']}</span>
                                )}
                                {responseHeaders['ratelimit-limit'] && (
                                  <span>Limit: {responseHeaders['ratelimit-limit']}</span>
                                )}
                                {responseHeaders['ratelimit-reset'] && (
                                  <span>Reset: {responseHeaders['ratelimit-reset']}s</span>
                                )}
                              </div>
                            </div>

                            {/* Response Body */}
                            <pre className="p-4 bg-graphite text-graphite-ink font-mono text-[10px] max-h-56 overflow-y-auto whitespace-pre-wrap">
                              {responseBody || 'No response body.'}
                            </pre>
                          </div>
                        )}
                      </div>
                    )}

                    {/* TAB: USAGE DOCS */}
                    {serviceDetailsTab === 'docs' && (
                      <div className="space-y-4">
                        <div className="rounded-control bg-paper-2 p-4 border border-rule text-xs flex gap-3">
                          <Info className="w-5 h-5 text-accent flex-shrink-0 mt-0.5" />
                          <div>
                            <p className="font-semibold text-ink">API Guides & Notes</p>
                            <p className="text-muted mt-0.5">
                              Hand-written usage guides and references for this OGC/REST API.
                            </p>
                          </div>
                        </div>
                        <p className="text-xs text-muted text-center py-6">No custom usage docs published for this service.</p>
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>
          </div>
        )}

        {/* ================================= API KEYS TAB ================================= */}
        {activeTab === 'keys' && (
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
            {/* Create new key card */}
            <div className="bg-paper border border-rule rounded-surface p-6 shadow-sm lg:col-span-1 h-fit">
              <h2 className="font-title text-base font-semibold text-ink flex items-center gap-2 mb-4 border-b border-rule pb-2">
                <Plus className="w-5 h-5 text-accent" /> Issue API Key
              </h2>

              {keySuccess && (
                <div className="flex items-start gap-3 rounded-control border border-ok-edge bg-ok-wash p-4 text-xs text-ok mb-4">
                  <CheckCircle className="w-4 h-4 flex-shrink-0 mt-0.5" />
                  <div>{keySuccess}</div>
                </div>
              )}

              {keyError && (
                <div className="flex items-start gap-3 rounded-control border border-danger-edge bg-danger-wash p-4 text-xs text-danger mb-4">
                  <AlertTriangle className="w-4 h-4 flex-shrink-0 mt-0.5" />
                  <div>{keyError}</div>
                </div>
              )}

              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  createKeyMutation.mutate();
                }}
                className="space-y-4"
              >
                <div>
                  <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                    Key Name
                  </label>
                  <input
                    type="text"
                    required
                    value={newKeyName}
                    onChange={(e) => setNewKeyName(e.target.value)}
                    placeholder="e.g. My App Key"
                    className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus font-sans"
                  />
                </div>

                <div>
                  <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                    Expiry Date (Optional)
                  </label>
                  <input
                    type="date"
                    value={newKeyExpiry}
                    onChange={(e) => setNewKeyExpiry(e.target.value)}
                    className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus font-sans"
                  />
                </div>

                <div className="pt-2">
                  <button
                    type="submit"
                    disabled={createKeyMutation.isPending}
                    className="w-full py-2 bg-accent hover:bg-accent-deep text-accent-ink text-sm font-semibold rounded-control transition duration-short"
                  >
                    {createKeyMutation.isPending ? 'Generating...' : 'Issue New Key'}
                  </button>
                </div>
              </form>
            </div>

            {/* Keys list and Copy section */}
            <div className="lg:col-span-2 space-y-6">
              {/* One-time secret display area */}
              {createdKey && (
                <div className="bg-ok-wash border-2 border-ok rounded-surface p-6 shadow-sm relative overflow-hidden">
                  <div className="absolute right-4 top-4 text-ok opacity-15">
                    <Sparkles className="w-16 h-16" />
                  </div>
                  <h3 className="font-title text-base font-bold text-ok flex items-center gap-2 mb-2">
                    <Sparkles className="w-5 h-5" /> Copy your API Key
                  </h3>
                  <p className="text-xs text-ink-2 mb-4">
                    For security reasons, this key will only be shown **once**. If you navigate away or refresh, you will not be able to retrieve it again.
                  </p>
                  
                  <div className="flex gap-2">
                    <pre className="flex-1 p-3 bg-paper border border-ok-edge rounded-control font-mono text-sm text-ink select-all break-all overflow-x-auto">
                      {createdKey}
                    </pre>
                    <button
                      onClick={() => copyToClipboard(createdKey, () => {})}
                      className="px-4 py-2 bg-ok hover:bg-ok-edge text-paper text-sm font-semibold rounded-control transition duration-short"
                    >
                      Copy
                    </button>
                  </div>
                </div>
              )}

              {/* Active keys table */}
              <div className="bg-paper border border-rule rounded-surface p-6 shadow-sm">
                <h2 className="font-title text-base font-semibold text-ink mb-4 border-b border-rule pb-2">
                  Active Keys
                </h2>

                {isLoadingKeys ? (
                  <div className="space-y-4">
                    {[...Array(3)].map((_, i) => (
                      <div key={i} className="h-12 bg-paper-2 rounded-control animate-pulse"></div>
                    ))}
                  </div>
                ) : !apiKeys || apiKeys.length === 0 ? (
                  <div className="text-center py-8 text-xs text-muted">
                    No active API keys found. Issue a key on the left to start calling APIs.
                  </div>
                ) : (
                  <div className="overflow-x-auto">
                    <table className="w-full text-left border-collapse text-xs">
                      <thead>
                        <tr className="border-b border-rule text-muted text-[10px] uppercase font-bold tracking-wider">
                          <th className="py-2 px-3">Name</th>
                          <th className="py-2 px-3">Expires At</th>
                          <th className="py-2 px-3">Created At</th>
                          <th className="py-2 px-3 text-right">Actions</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-rule">
                        {apiKeys.map((key) => (
                          <tr key={key.id} className="hover:bg-paper-2 transition-colors">
                            <td className="py-3 px-3 font-semibold text-ink">{key.name}</td>
                            <td className="py-3 px-3 text-muted">
                              {key.expiredAt ? (
                                <span className="flex items-center gap-1">
                                  <Calendar className="w-3.5 h-3.5" />
                                  {new Date(key.expiredAt).toLocaleDateString()}
                                </span>
                              ) : (
                                'Never expires'
                              )}
                            </td>
                            <td className="py-3 px-3 text-muted">
                              {key.createdAt ? new Date(key.createdAt).toLocaleDateString() : 'N/A'}
                            </td>
                            <td className="py-3 px-3 text-right space-x-1">
                              <button
                                onClick={() => {
                                  if (confirm('Regenerate key? The old key value will stop working immediately.')) {
                                    regenerateKeyMutation.mutate(key.id);
                                  }
                                }}
                                disabled={regenerateKeyMutation.isPending}
                                className="px-2.5 py-1 bg-paper-3 hover:bg-rule border border-rule hover:border-rule-2 text-[10px] font-semibold rounded-chip transition duration-short"
                              >
                                Regenerate
                              </button>
                              <button
                                onClick={() => {
                                  if (confirm('Delete this API key permanently? Any application using this key will fail to make requests.')) {
                                    deleteKeyMutation.mutate(key.id);
                                  }
                                }}
                                disabled={deleteKeyMutation.isPending}
                                className="p-1 rounded-chip text-muted hover:text-danger hover:bg-danger-wash border border-transparent hover:border-danger-edge transition duration-short inline-flex items-center align-middle"
                                title="Delete Key"
                              >
                                <Trash2 className="w-4 h-4" />
                              </button>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
            </div>
          </div>
        )}
      </div>
    </NavigationShell>
  );
}
