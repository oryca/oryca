/* Hallmark · genre: modern-minimal · macrostructure: Workbench
 * design-system: design.md · designed-as-app
 */
'use client';

import React, { useEffect, useRef, useState } from 'react';
import {
  useQuery,
  useMutation,
  useQueryClient,
  type UseMutationResult,
} from '@tanstack/react-query';
import { api, API_URL } from '@/lib/api';
import { useInfiniteList } from '@/lib/useInfiniteList';
import NavigationShell from '@/components/NavigationShell';
import {
  Button,
  PageHeader,
  EmptyState,
  SkeletonLine,
  Loading,
  useToast,
  useConfirm,
  InfiniteScrollSentinel,
} from '@/components/ui';
import {
  Bell,
  CheckCheck,
  AlertTriangle,
  Info,
  Clock,
  Trash2,
  Wifi,
  WifiOff,
} from 'lucide-react';

interface Notification {
  id: string;
  topic: 'apikey_suspended' | 'package_changed' | 'package_services_updated' | 'apikey_expiring';
  level: 'info' | 'warning' | 'error';
  title: string;
  message: string;
  read: boolean;
  createdAt: string;
}

/** สีของแถวมาจาก level ก่อน ถ้ายังไม่อ่านค่อยใช้ accent */
function toneOf(n: Notification) {
  if (n.level === 'warning') {
    return { surface: 'border-warn-edge bg-warn-wash', icon: 'text-warn', Icon: AlertTriangle };
  }
  if (n.level === 'error') {
    return { surface: 'border-danger-edge bg-danger-wash', icon: 'text-danger', Icon: AlertTriangle };
  }
  if (!n.read) {
    return { surface: 'border-accent-edge bg-accent-wash', icon: 'text-accent', Icon: Info };
  }
  return { surface: 'border-rule bg-paper', icon: 'text-muted', Icon: Info };
}

function StreamStatus({ connected }: { connected: boolean }) {
  return (
    <span
      aria-live="polite"
      className={`ui-status ${
        connected ? 'border-ok-edge bg-ok-wash text-ok' : 'border-warn-edge bg-warn-wash text-warn'
      }`}
    >
      {connected ? (
        <>
          <Wifi className="h-4 w-4" aria-hidden="true" /> Live
        </>
      ) : (
        <>
          <WifiOff className="h-4 w-4" aria-hidden="true" /> Reconnecting
        </>
      )}
    </span>
  );
}

function NotificationSkeleton() {
  return (
    <Loading label="Loading notifications">
      <div className="space-y-3">
        {[0, 1, 2].map((i) => (
          <div key={i} className="ui-card p-4">
            <SkeletonLine width="35%" className="mb-2 h-4" />
            <SkeletonLine width="80%" />
            <SkeletonLine width="25%" className="mt-2" />
          </div>
        ))}
      </div>
    </Loading>
  );
}

export default function NotificationsPage() {
  const queryClient = useQueryClient();
  const { error: toastError } = useToast();
  const confirm = useConfirm();
  const [sseConnected, setSseConnected] = useState(false);
  const listScrollRef = useRef<HTMLDivElement>(null);

  const {
    items: notifications,
    isLoading,
    hasNextPage,
    isFetchingNextPage,
    fetchNextPage,
  } = useInfiniteList<Notification>(['notifications'], '/notifications');

  // The badge counts every unread one, not just the pages loaded so far.
  const { data: unreadCount = 0 } = useQuery<number>({
    queryKey: ['notifications', 'unread-count'],
    queryFn: async () => {
      const res = await api.get('/notifications/unread-count');
      return res.data?.count ?? 0;
    },
  });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['notifications'] });

  // ระบุชนิดตรงๆ เพราะ onError อ้างถึงตัว mutation เองเพื่อทำปุ่ม "ลองใหม่"
  // ถ้าปล่อยให้ infer จะวนกลับมาหาตัวเอง (TS7022)
  const markAllRead: UseMutationResult<void, Error, void> = useMutation({
    mutationFn: async () => {
      await api.put('/notifications/read-all');
    },
    onSuccess: invalidate,
    // ผลเห็นได้ทันทีบนหน้าจอ จึงไม่ต้อง toast ตอนสำเร็จ
    onError: () => toastError('Could not mark everything as read', () => markAllRead.mutate()),
  });

  const markRead: UseMutationResult<void, Error, string> = useMutation({
    mutationFn: async (id: string) => {
      await api.put(`/notifications/${id}/read`);
    },
    onSuccess: invalidate,
    onError: (_e, id) => toastError('Could not mark that as read', () => markRead.mutate(id)),
  });

  const removeOne: UseMutationResult<void, Error, string> = useMutation({
    mutationFn: async (id: string) => {
      await api.delete(`/notifications/${id}`);
    },
    onSuccess: invalidate,
    onError: (_e, id) => toastError('Could not delete the notification', () => removeOne.mutate(id)),
  });

  async function askDelete(n: Notification) {
    const ok = await confirm({
      title: 'Delete this notification',
      description: n.title,
      consequences: ['This cannot be undone', 'Other notifications are untouched'],
      confirmLabel: 'Delete',
      danger: true,
    });
    if (ok) removeOne.mutate(n.id);
  }

  // SSE: ขอ ticket แล้วเปิด stream ต่อใหม่อัตโนมัติถ้าหลุด
  useEffect(() => {
    let eventSource: EventSource | null = null;
    let isActive = true;
    let retry: ReturnType<typeof setTimeout> | undefined;

    async function setupSSE() {
      try {
        const ticketRes = await api.post('/notifications/stream-ticket');
        const ticket = ticketRes.data.ticket;
        if (!isActive) return;

        eventSource = new EventSource(`${API_URL}/notifications/stream?ticket=${ticket}`);

        eventSource.onopen = () => {
          if (isActive) setSseConnected(true);
        };

        eventSource.onerror = () => {
          if (isActive) setSseConnected(false);
          eventSource?.close();
          retry = setTimeout(() => {
            if (isActive) setupSSE();
          }, 5000);
        };

        eventSource.onmessage = () => {
          queryClient.invalidateQueries({ queryKey: ['notifications'] });
        };
      } catch {
        if (isActive) setSseConnected(false);
        retry = setTimeout(() => {
          if (isActive) setupSSE();
        }, 5000);
      }
    }

    setupSSE();

    return () => {
      isActive = false;
      if (retry) clearTimeout(retry);
      eventSource?.close();
    };
  }, [queryClient]);

  return (
    <NavigationShell>
      <PageHeader
        title="Notifications"
        description="Live updates about your API keys, package and rate limits"
        actions={
          <>
            <StreamStatus connected={sseConnected} />
            <Button
              icon={<CheckCheck className="h-4 w-4" />}
              onClick={() => markAllRead.mutate()}
              loading={markAllRead.isPending}
              disabled={unreadCount === 0}
              title={unreadCount === 0 ? 'Everything is read' : undefined}
            >
              Mark all read
            </Button>
          </>
        }
      />

      {isLoading && <NotificationSkeleton />}

      {!isLoading && notifications.length === 0 && (
        <div className="ui-card">
          <EmptyState
            icon={<Bell className="h-5 w-5" />}
            title="Nothing to read"
            description="You will hear here when a key is about to expire, gets suspended, or your package changes."
          />
        </div>
      )}

      {!isLoading && notifications.length > 0 && (
        <div ref={listScrollRef} className="max-h-[min(840px,70vh)] overflow-y-auto">
        <ul className="ui-fade space-y-3">
          {notifications.map((n) => {
            const tone = toneOf(n);
            const Icon = tone.Icon;

            return (
              <li
                key={n.id}
                className={`flex items-start justify-between gap-3 rounded-surface border p-4 ${tone.surface}`}
              >
                <div className="flex min-w-0 items-start gap-3">
                  <span
                    aria-hidden="true"
                    className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-control border border-rule bg-paper ${tone.icon}`}
                  >
                    <Icon className="h-4 w-4" />
                  </span>

                  <div className="min-w-0">
                    <p
                      className={`text-sm font-semibold ${n.read ? 'text-ink-2' : 'text-ink'}`}
                    >
                      {n.title}
                      {!n.read && (
                        <span className="ml-2 align-middle text-xs font-normal text-accent">
                          New
                        </span>
                      )}
                    </p>
                    <p className="mt-1 text-sm leading-relaxed text-ink-3">{n.message}</p>
                    <p className="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted">
                      <span className="inline-flex items-center gap-1">
                        <Clock className="h-3 w-3" aria-hidden="true" />
                        {new Date(n.createdAt).toLocaleString()}
                      </span>
                      <span aria-hidden="true">·</span>
                      <span className="font-mono">{n.topic}</span>
                    </p>
                  </div>
                </div>

                <div className="flex shrink-0 items-center gap-1">
                  {!n.read && (
                    <Button
                      size="sm"
                      onClick={() => markRead.mutate(n.id)}
                      loading={markRead.isPending && markRead.variables === n.id}
                    >
                      Mark read
                    </Button>
                  )}
                  <Button
                    size="sm"
                    variant="ghost-danger"
                    onClick={() => askDelete(n)}
                    aria-label={`Delete notification: ${n.title}`}
                    icon={<Trash2 className="h-4 w-4" />}
                  />
                </div>
              </li>
            );
          })}
        </ul>
        <InfiniteScrollSentinel
          rootRef={listScrollRef}
          hasNextPage={hasNextPage}
          isFetchingNextPage={isFetchingNextPage}
          fetchNextPage={fetchNextPage}
        />
        </div>
      )}
    </NavigationShell>
  );
}
