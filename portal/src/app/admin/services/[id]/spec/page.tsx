/* Hallmark · genre: modern-minimal · macrostructure: Workbench
 * design-system: design.md · designed-as-app
 */
'use client';

import React, { useState } from 'react';
import { useQuery, useMutation, type UseMutationResult } from '@tanstack/react-query';
import { useParams, useRouter } from 'next/navigation';
import axios from 'axios';
import { api } from '@/lib/api';
import NavigationShell from '@/components/NavigationShell';
import {
  Button,
  Field,
  TextAreaField,
  PageHeader,
  SectionCard,
  Notice,
  SkeletonLine,
  Loading,
  useToast,
  useConfirm,
} from '@/components/ui';
import { ArrowLeft, Save, Trash2 } from 'lucide-react';

interface SpecPath {
  path: string;
  method: string;
  operationId?: string;
  tags?: string[];
  summary?: string;
  description?: string;
  parameters?: unknown[];
  requestBody?: unknown;
  responses?: unknown[];
}

interface GatewaySpec {
  version?: string;
  serverDescription?: string;
  paths?: SpecPath[];
}

interface ServiceDetail {
  name?: string;
  resourcePaths?: Array<{ path: string; methods: string[] }>;
}

function messageOf(err: unknown, fallback: string) {
  return err instanceof Error && err.message ? err.message : fallback;
}

/** สร้างโครงร่างจากเส้นทางที่ service มีอยู่ ให้ผู้ดูแลแก้ต่อแทนที่จะเริ่มจากศูนย์ */
function scaffoldFrom(service: ServiceDetail | undefined): SpecPath[] {
  return (service?.resourcePaths ?? []).map((rp) => ({
    path: rp.path,
    method: rp.methods[0] || 'GET',
    summary: `Endpoint for ${rp.path}`,
    description: `Call this endpoint to access ${rp.path}`,
    parameters: [],
    responses: [
      {
        status: 200,
        description: 'Success response',
        content: { 'application/json': { schema: { type: 'object' } } },
      },
    ],
  }));
}

export default function AdminSpecEditorPage() {
  const router = useRouter();
  const { id: serviceId } = useParams<{ id: string }>();
  const { toast, error: toastError } = useToast();
  const confirm = useConfirm();

  const [version, setVersion] = useState('1.0.0');
  const [serverDescription, setServerDescription] = useState('');
  const [pathsJson, setPathsJson] = useState('[]');

  const { data: service, isLoading: isLoadingService } = useQuery<ServiceDetail>({
    queryKey: ['admin-service-detail', serviceId],
    queryFn: async () => (await api.get(`/services/${serviceId}`)).data,
    enabled: !!serviceId,
  });

  const { data: specData, isLoading: isLoadingSpec } = useQuery<GatewaySpec>({
    queryKey: ['admin-service-spec', serviceId],
    queryFn: async () => {
      try {
        const res = await api.get(`/services/${serviceId}/spec`);
        return res.data;
      } catch (err) {
        // 404 = ยังไม่เคยเขียนสเปก ไม่ใช่ความผิดพลาด
        if (axios.isAxiosError(err) && err.response?.status === 404) return {};
        throw err;
      }
    },
    enabled: !!serviceId,
    retry: false,
  });

  const hasSavedSpec = Boolean(specData?.paths && specData.paths.length > 0);
  const isLoading = isLoadingSpec || isLoadingService;

  // เติมฟอร์มครั้งเดียวหลังทั้งสอง query นิ่งแล้ว ปรับ state ระหว่าง render ไม่ใช่ใน effect
  const [loadedFor, setLoadedFor] = useState<string | null>(null);
  if (!isLoading && serviceId && loadedFor !== serviceId) {
    setLoadedFor(serviceId);
    setVersion(specData?.version || '1.0.0');
    setServerDescription(specData?.serverDescription || '');
    const paths =
      specData?.paths && specData.paths.length > 0 ? specData.paths : scaffoldFrom(service);
    setPathsJson(JSON.stringify(paths, null, 2));
  }

  // ตรวจ JSON ทันทีที่พิมพ์ ไม่ต้องรอกดบันทึกถึงจะรู้ว่าพัง
  let jsonError: string | null = null;
  try {
    const parsed = JSON.parse(pathsJson);
    if (!Array.isArray(parsed)) jsonError = 'This has to be an array of paths, not an object.';
  } catch (err) {
    jsonError = messageOf(err, 'That is not valid JSON.');
  }

  const saveSpec: UseMutationResult<void, Error, void> = useMutation({
    mutationFn: async () => {
      await api.put(`/services/${serviceId}/spec`, {
        version,
        serverDescription,
        paths: JSON.parse(pathsJson) as SpecPath[],
      });
    },
    // ผลของการบันทึกมองไม่เห็นบนหน้าจอ จึงต้องบอก
    onSuccess: () => toast({ tone: 'ok', message: 'Spec saved' }),
    onError: (err) => toastError(messageOf(err, 'Could not save the spec')),
  });

  const deleteSpec: UseMutationResult<void, Error, void> = useMutation({
    mutationFn: async () => {
      await api.delete(`/services/${serviceId}/spec`);
    },
    onSuccess: () => {
      toast({ tone: 'ok', message: 'Spec deleted' });
      setVersion('1.0.0');
      setServerDescription('');
      setPathsJson('[]');
    },
    onError: (err) => toastError(messageOf(err, 'Could not delete the spec')),
  });

  async function askDelete() {
    const ok = await confirm({
      title: 'Delete this service spec',
      description: service?.name,
      consequences: [
        'This cannot be undone',
        'The service drops out of the OpenAPI catalog users browse',
        'The service itself keeps serving requests through the gateway',
      ],
      confirmLabel: 'Delete spec',
      danger: true,
    });
    if (ok) deleteSpec.mutate();
  }

  return (
    <NavigationShell>
      <div className="mb-4">
        <Button
          size="sm"
          variant="ghost"
          icon={<ArrowLeft className="h-4 w-4" />}
          onClick={() => router.push('/admin/services')}
        >
          Back to Manage Services
        </Button>
      </div>

      <PageHeader
        title="Service spec"
        description={`The OpenAPI manual for ${service?.name || 'this service'} - it builds the catalog users browse`}
        actions={
          hasSavedSpec && (
            <Button
              variant="danger"
              onClick={askDelete}
              loading={deleteSpec.isPending}
              icon={<Trash2 className="h-4 w-4" />}
            >
              Delete spec
            </Button>
          )
        }
      />

      {isLoading && (
        <Loading label="Loading spec">
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
            <div className="ui-card space-y-3 p-4">
              <SkeletonLine width="50%" className="h-4" />
              <SkeletonLine className="h-10" />
              <SkeletonLine className="h-20" />
            </div>
            <div className="ui-card p-4 lg:col-span-2">
              <SkeletonLine width="40%" className="mb-3 h-4" />
              <SkeletonLine className="h-64" />
            </div>
          </div>
        </Loading>
      )}

      {!isLoading && (
        <form
          className="grid grid-cols-1 gap-6 lg:grid-cols-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (!jsonError) saveSpec.mutate();
          }}
        >
          <div className="lg:col-span-1">
            <SectionCard title="Metadata">
              <div className="space-y-4">
                <Field
                  label="Spec version"
                  value={version}
                  onChange={(e) => setVersion(e.target.value)}
                  placeholder="1.0.0"
                  inputClassName="font-mono"
                  required
                />

                <TextAreaField
                  label="Server description"
                  rows={4}
                  value={serverDescription}
                  onChange={(e) => setServerDescription(e.target.value)}
                  placeholder="e.g. production OGC features endpoint"
                />

                <Button
                  type="submit"
                  variant="primary"
                  block
                  loading={saveSpec.isPending}
                  disabled={Boolean(jsonError)}
                  icon={<Save className="h-4 w-4" />}
                >
                  Save spec
                </Button>
              </div>
            </SectionCard>
          </div>

          <div className="lg:col-span-2">
            <SectionCard
              title="Paths and endpoints"
              description="Format: an array of SpecPath"
            >
              <div className="space-y-3">
                {!hasSavedSpec && (
                  <Notice tone="info">
                    No spec saved yet, so this is scaffolded from the routes the service already has.
                    Edit parameters, requestBody and response shapes below.
                  </Notice>
                )}

                {jsonError && <Notice tone="danger">{jsonError}</Notice>}

                <div>
                  <label htmlFor="paths-json" className="ui-field__label">
                    Paths JSON
                  </label>
                  <textarea
                    id="paths-json"
                    rows={20}
                    required
                    aria-invalid={jsonError ? true : undefined}
                    value={pathsJson}
                    onChange={(e) => setPathsJson(e.target.value)}
                    className="ui-input w-full bg-graphite p-4 font-mono text-xs text-graphite-ink"
                  />
                </div>
              </div>
            </SectionCard>
          </div>
        </form>
      )}
    </NavigationShell>
  );
}
