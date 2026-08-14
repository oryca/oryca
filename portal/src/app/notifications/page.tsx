'use client';

import React, { useEffect, useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, NEXT_PUBLIC_API_URL } from '@/lib/api';
import NavigationShell from '@/components/NavigationShell';
import {
  Bell,
  CheckCheck,
  AlertTriangle,
  Info,
  Clock,
  Trash2,
  Wifi,
  WifiOff
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

export default function NotificationsPage() {
  const queryClient = useQueryClient();
  const [sseConnected, setSseConnected] = useState(false);

  // Fetch Notifications
  const { data: notifications, isLoading, refetch } = useQuery<Notification[]>({
    queryKey: ['notifications'],
    queryFn: async () => {
      const res = await api.get('/notifications');
      return res.data.items || [];
    },
  });

  // Mark all read mutation
  const markAllReadMutation = useMutation({
    mutationFn: async () => {
      await api.put('/notifications/read-all');
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notifications'] });
    },
  });

  // Mark single read mutation
  const markReadMutation = useMutation({
    mutationFn: async (id: string) => {
      await api.put(`/notifications/${id}/read`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notifications'] });
    },
  });

  // Delete notification mutation
  const deleteNotificationMutation = useMutation({
    mutationFn: async (id: string) => {
      await api.delete(`/notifications/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notifications'] });
    },
  });

  // SSE streaming ticket listener
  useEffect(() => {
    let eventSource: EventSource | null = null;
    let isActive = true;

    async function setupSSE() {
      try {
        // Step 1: Request stream ticket
        const ticketRes = await api.post('/notifications/stream-ticket');
        const ticket = ticketRes.data.ticket;

        if (!isActive) return;

        // Step 2: Open EventSource
        const sseUrl = `${NEXT_PUBLIC_API_URL}/notifications/stream?ticket=${ticket}`;
        eventSource = new EventSource(sseUrl);

        eventSource.onopen = () => {
          if (isActive) setSseConnected(true);
        };

        eventSource.onerror = (e) => {
          console.error('SSE connection error:', e);
          if (isActive) setSseConnected(false);
          eventSource?.close();
          // Attempt reconnect after 5s
          setTimeout(() => {
            if (isActive) setupSSE();
          }, 5000);
        };

        eventSource.onmessage = (event) => {
          console.log('SSE notification received:', event.data);
          // Invalidate notifications list query to refresh UI
          queryClient.invalidateQueries({ queryKey: ['notifications'] });
        };
      } catch (err) {
        console.error('Failed to setup SSE notifications stream:', err);
        if (isActive) setSseConnected(false);
        // Attempt reconnect after 5s
        setTimeout(() => {
          if (isActive) setupSSE();
        }, 5000);
      }
    }

    setupSSE();

    return () => {
      isActive = false;
      if (eventSource) {
        eventSource.close();
      }
    };
  }, [queryClient]);

  return (
    <NavigationShell>
      <div className="space-y-6">
        {/* Header */}
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
          <div>
            <h1 className="font-title text-2xl font-bold tracking-tight text-ink flex items-center gap-2">
              Notifications
            </h1>
            <p className="text-sm text-muted">
              Live updates and diagnostics regarding your API keys and rate limits
            </p>
          </div>

          <div className="flex items-center gap-4">
            {/* Live SSE Status Badge */}
            <div className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-chip text-[10px] font-bold border ${
              sseConnected
                ? 'text-ok bg-ok-wash border-ok-edge'
                : 'text-warn bg-warn-wash border-warn-edge'
            }`}>
              {sseConnected ? (
                <>
                  <Wifi className="w-3.5 h-3.5" /> Live connected
                </>
              ) : (
                <>
                  <WifiOff className="w-3.5 h-3.5" /> Reconnecting
                </>
              )}
            </div>

            <button
              onClick={() => markAllReadMutation.mutate()}
              disabled={markAllReadMutation.isPending || !notifications || notifications.filter(n => !n.read).length === 0}
              className="px-3 py-1.5 border border-rule hover:border-faint rounded-control text-xs font-semibold text-ink-2 hover:bg-paper-2 transition flex items-center gap-2 disabled:opacity-50"
            >
              <CheckCheck className="w-3.5 h-3.5" />
              Mark all read
            </button>
          </div>
        </div>

        {/* Notifications list */}
        {isLoading ? (
          <div className="space-y-3">
            {[...Array(3)].map((_, i) => (
              <div key={i} className="h-16 bg-paper rounded-surface border border-rule animate-pulse"></div>
            ))}
          </div>
        ) : !notifications || notifications.length === 0 ? (
          <div className="p-12 bg-paper border border-rule border-dashed rounded-surface flex flex-col items-center justify-center text-muted text-center gap-2 shadow-sm">
            <Bell className="w-8 h-8 opacity-40 text-muted" />
            <p className="text-xs">You have no notifications yet.</p>
          </div>
        ) : (
          <div className="space-y-3">
            {notifications.map((notification) => {
              // Level colors
              let bgBorderClass = 'bg-paper border-rule';
              let Icon = Info;
              let iconColor = 'text-muted';

              if (notification.level === 'warning') {
                bgBorderClass = 'bg-warn-wash border-warn-edge';
                Icon = AlertTriangle;
                iconColor = 'text-warn';
              } else if (notification.level === 'error') {
                bgBorderClass = 'bg-danger-wash border-danger-edge';
                Icon = AlertTriangle;
                iconColor = 'text-danger';
              } else if (!notification.read) {
                bgBorderClass = 'bg-accent-wash border-accent-edge';
                iconColor = 'text-accent';
              }

              return (
                <div
                  key={notification.id}
                  className={`p-4 rounded-surface border flex items-start justify-between gap-4 transition shadow-sm ${bgBorderClass} ${
                    notification.read ? 'opacity-80 hover:opacity-100' : ''
                  }`}
                >
                  <div className="flex items-start gap-3">
                    <div className={`p-2 rounded-control bg-paper flex items-center justify-center border border-rule ${iconColor}`}>
                      <Icon className="w-4 h-4" />
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <p className={`text-sm font-semibold ${notification.read ? 'text-ink-2' : 'text-ink'}`}>
                          {notification.title}
                        </p>
                        {!notification.read && (
                          <span className="w-1.5 h-1.5 rounded-full bg-accent"></span>
                        )}
                      </div>
                      <p className="text-xs text-ink-3 mt-1 leading-relaxed">{notification.message}</p>
                      <span className="text-[10px] text-muted flex items-center gap-1 mt-2">
                        <Clock className="w-3 h-3" />
                        {new Date(notification.createdAt).toLocaleString()} • Topic: {notification.topic}
                      </span>
                    </div>
                  </div>

                  <div className="flex items-center gap-1">
                    {!notification.read && (
                      <button
                        onClick={() => markReadMutation.mutate(notification.id)}
                        className="px-2 py-1 bg-paper border border-rule hover:border-faint text-[10px] font-semibold rounded-chip text-ink-2 hover:bg-paper-2 transition"
                        title="Mark as read"
                      >
                        Mark read
                      </button>
                    )}
                    <button
                      onClick={() => {
                        if (confirm('Delete this notification permanently?')) {
                          deleteNotificationMutation.mutate(notification.id);
                        }
                      }}
                      className="p-1.5 rounded-control text-muted hover:text-danger hover:bg-danger-wash border border-transparent hover:border-danger-edge transition"
                      title="Delete Notification"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </NavigationShell>
  );
}
