'use client';

import React, { useState, useEffect } from 'react';
import { useQuery, useMutation } from '@tanstack/react-query';
import { useParams, useRouter } from 'next/navigation';
import { api } from '@/lib/api';
import NavigationShell from '@/components/NavigationShell';
import {
  BookOpen,
  ArrowLeft,
  CheckCircle,
  AlertTriangle,
  Save,
  Trash2,
  FileCode,
  Info
} from 'lucide-react';

interface SpecPath {
  path: string;
  method: string;
  operationId?: string;
  tags?: string[];
  summary?: string;
  description?: string;
  parameters?: any[];
  requestBody?: any;
  responses?: any[];
}

interface GatewaySpec {
  version?: string;
  serverDescription?: string;
  paths?: SpecPath[];
}

export default function AdminSpecEditorPage() {
  const router = useRouter();
  const { id: serviceId } = useParams();

  const [version, setVersion] = useState('1.0.0');
  const [serverDescription, setServerDescription] = useState('');
  const [pathsJson, setPathsJson] = useState('[]');
  
  const [success, setSuccess] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Fetch Service Details
  const { data: service } = useQuery({
    queryKey: ['admin-service-detail', serviceId],
    queryFn: async () => {
      const res = await api.get(`/services/${serviceId}`);
      return res.data;
    },
    enabled: !!serviceId,
  });

  // Fetch Service Spec (might return 404 if not created)
  const { data: specData, isLoading: isLoadingSpec } = useQuery<GatewaySpec>({
    queryKey: ['admin-service-spec', serviceId],
    queryFn: async () => {
      try {
        const res = await api.get(`/services/${serviceId}/spec`);
        return res.data;
      } catch (err: any) {
        if (err.response?.status === 404) {
          return {}; // Return empty if not found
        }
        throw err;
      }
    },
    enabled: !!serviceId,
    retry: false,
  });

  // Load spec into states
  useEffect(() => {
    if (specData) {
      setVersion(specData.version || '1.0.0');
      setServerDescription(specData.serverDescription || '');
      if (specData.paths && specData.paths.length > 0) {
        setPathsJson(JSON.stringify(specData.paths, null, 2));
      } else if (service?.resourcePaths) {
        // Auto scaffold template paths from the service resource paths!
        const templatePaths: SpecPath[] = service.resourcePaths.map((rp: any) => ({
          path: rp.path,
          method: rp.methods[0] || 'GET',
          summary: `Endpoint for ${rp.path}`,
          description: `Call this endpoint to access ${rp.path} services`,
          parameters: [],
          responses: [
            {
              status: 200,
              description: 'Success response',
              content: {
                'application/json': {
                  schema: { type: 'object' }
                }
              }
            }
          ]
        }));
        setPathsJson(JSON.stringify(templatePaths, null, 2));
      }
    }
  }, [specData, service]);

  const saveMutation = useMutation({
    mutationFn: async () => {
      setSuccess(null);
      setError(null);
      
      // Parse paths JSON
      let parsedPaths: SpecPath[] = [];
      try {
        parsedPaths = JSON.parse(pathsJson);
        if (!Array.isArray(parsedPaths)) {
          throw new Error('Paths spec must be an array.');
        }
      } catch (err: any) {
        throw new Error(`Invalid JSON format in Paths Specification: ${err.message}`);
      }

      const payload = {
        version,
        serverDescription,
        paths: parsedPaths,
      };

      await api.put(`/services/${serviceId}/spec`, payload);
    },
    onSuccess: () => {
      setSuccess('Service OpenAPI manual saved successfully.');
    },
    onError: (err: any) => {
      setError(err.message || 'Failed to save spec.');
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async () => {
      setSuccess(null);
      setError(null);
      await api.delete(`/services/${serviceId}/spec`);
    },
    onSuccess: () => {
      setSuccess('Specification deleted.');
      setVersion('1.0.0');
      setServerDescription('');
      setPathsJson('[]');
    },
    onError: (err: any) => {
      setError(err.message || 'Failed to delete specification.');
    },
  });

  return (
    <NavigationShell>
      <div className="space-y-6">
        {/* Back button */}
        <div>
          <button
            onClick={() => router.push('/admin/services')}
            className="flex items-center gap-1.5 text-xs font-semibold text-accent hover:underline"
          >
            <ArrowLeft className="w-3.5 h-3.5" /> Back to Services
          </button>
        </div>

        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
          <div>
            <h1 className="font-title text-2xl font-bold tracking-tight text-ink flex items-center gap-2">
              <BookOpen className="w-6 h-6 text-accent" /> Service Spec Manual
            </h1>
            <p className="text-sm text-muted">
              Write the OpenAPI manual for {service?.name || 'Service'}. This is used to build the gateway OpenAPI catalog.
            </p>
          </div>

          {specData && (
            <button
              onClick={() => {
                if (confirm('Delete this specification permanently?')) {
                  deleteMutation.mutate();
                }
              }}
              className="text-xs text-danger font-semibold hover:underline"
            >
              Delete Spec
            </button>
          )}
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

        {isLoadingSpec ? (
          <div className="flex justify-center p-12">
            <div className="w-8 h-8 border-2 border-rule border-t-accent rounded-full animate-spin"></div>
          </div>
        ) : (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              saveMutation.mutate();
            }}
            className="grid grid-cols-1 lg:grid-cols-3 gap-6 text-xs"
          >
            {/* Core details */}
            <div className="lg:col-span-1 bg-paper border border-rule rounded-surface p-6 shadow-sm space-y-4 h-fit">
              <h2 className="font-title text-sm font-semibold text-ink border-b border-rule pb-2">
                Metadata Settings
              </h2>

              <div>
                <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                  API Spec Version
                </label>
                <input
                  type="text"
                  required
                  value={version}
                  onChange={(e) => setVersion(e.target.value)}
                  placeholder="e.g. 1.0.0"
                  className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus font-mono"
                />
              </div>

              <div>
                <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                  Server Description
                </label>
                <textarea
                  rows={4}
                  value={serverDescription}
                  onChange={(e) => setServerDescription(e.target.value)}
                  placeholder="e.g. Production OGC gateway features endpoint"
                  className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                />
              </div>

              <div className="pt-2">
                <button
                  type="submit"
                  disabled={saveMutation.isPending}
                  className="w-full py-2.5 bg-accent hover:bg-accent-deep text-accent-ink text-sm font-semibold rounded-control transition duration-short flex items-center justify-center gap-2"
                >
                  <Save className="w-4 h-4" />
                  {saveMutation.isPending ? 'Saving spec...' : 'Save Manual Specification'}
                </button>
              </div>
            </div>

            {/* Paths JSON spec editor */}
            <div className="lg:col-span-2 bg-paper border border-rule rounded-surface p-6 shadow-sm space-y-4">
              <div className="flex items-center justify-between border-b border-rule pb-2">
                <h2 className="font-title text-sm font-semibold text-ink flex items-center gap-2">
                  <FileCode className="w-5 h-5 text-accent" /> Paths & Endpoints Spec (JSON)
                </h2>
                <span className="text-[10px] text-muted font-mono">Format: Array&lt;SpecPath&gt;</span>
              </div>

              <div className="bg-paper-2 p-3 border border-rule rounded-control text-[11px] text-muted flex gap-2">
                <Info className="w-4 h-4 text-accent mt-0.5 flex-shrink-0" />
                <p>
                  We auto-generated a boilerplate paths schema matching your service resource routes. Edit parameters, requestBody, and response structures directly in the JSON editor below.
                </p>
              </div>

              <div>
                <textarea
                  rows={20}
                  required
                  value={pathsJson}
                  onChange={(e) => setPathsJson(e.target.value)}
                  className="w-full bg-graphite text-graphite-ink border border-graphite-rule rounded-control p-4 font-mono text-[11px] outline-none focus:border-accent"
                />
              </div>
            </div>
          </form>
        )}
      </div>
    </NavigationShell>
  );
}
