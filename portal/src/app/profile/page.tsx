'use client';

import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, getAccessToken } from '@/lib/api';
import { useAuth } from '@/app/providers';
import NavigationShell from '@/components/NavigationShell';
import {
  User,
  KeyRound,
  ShieldAlert,
  Monitor,
  Globe,
  Clock,
  Trash2,
  AlertTriangle,
  CheckCircle,
  Eye,
  EyeOff
} from 'lucide-react';

interface Session {
  id: string;
  deviceInfo?: string;
  browser?: string;
  os?: string;
  ipAddress?: string;
  location?: string;
  loginAt: string;
}

export default function ProfilePage() {
  const queryClient = useQueryClient();
  const { user, refreshUser } = useAuth();
  
  // Profile update state
  const [firstName, setFirstName] = useState(user?.firstName || '');
  const [lastName, setLastName] = useState(user?.lastName || '');
  const [displayName, setDisplayName] = useState(user?.displayName || '');
  const [organization, setOrganization] = useState('');
  
  const [profileSuccess, setProfileSuccess] = useState<string | null>(null);
  const [profileError, setProfileError] = useState<string | null>(null);

  // Password change state
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [passwordSuccess, setPasswordSuccess] = useState<string | null>(null);
  const [passwordError, setPasswordError] = useState<string | null>(null);

  // Initialize profile inputs once user loads
  React.useEffect(() => {
    if (user) {
      setFirstName(user.firstName || '');
      setLastName(user.lastName || '');
      setDisplayName(user.displayName || '');
    }
  }, [user]);

  // Fetch active sessions
  const { data: sessions, isLoading: isLoadingSessions, refetch: refetchSessions } = useQuery<Session[]>({
    queryKey: ['sessions'],
    queryFn: async () => {
      const res = await api.get('/account/sessions');
      return res.data.items || [];
    },
  });

  // Profile update mutation
  const updateProfileMutation = useMutation({
    mutationFn: async () => {
      setProfileError(null);
      setProfileSuccess(null);
      const res = await api.put('/account/profile', {
        firstName,
        lastName,
        displayName,
        organization,
      });
      return res.data;
    },
    onSuccess: async () => {
      await refreshUser();
      setProfileSuccess('Profile updated successfully.');
    },
    onError: (err: any) => {
      setProfileError(err.message || 'Failed to update profile.');
    },
  });

  // Password change mutation
  const changePasswordMutation = useMutation({
    mutationFn: async () => {
      setPasswordError(null);
      setPasswordSuccess(null);
      if (newPassword !== confirmPassword) {
        throw new Error('New passwords do not match.');
      }
      const res = await api.post('/account/change-password', {
        currentPassword,
        newPassword,
      });
      return res.data;
    },
    onSuccess: () => {
      setPasswordSuccess('Password changed successfully.');
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
    },
    onError: (err: any) => {
      setPasswordError(err.message || 'Failed to change password.');
    },
  });

  // Revoke session mutation
  const revokeSessionMutation = useMutation({
    mutationFn: async (sessionId: string) => {
      await api.delete(`/account/sessions/${sessionId}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['sessions'] });
    },
  });

  // Revoke all other sessions mutation
  const revokeAllSessionsMutation = useMutation({
    mutationFn: async () => {
      await api.delete('/account/sessions');
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['sessions'] });
    },
  });

  return (
    <NavigationShell>
      <div className="space-y-6">
        <div>
          <h1 className="font-title text-2xl font-bold tracking-tight text-ink">
            Profile & Sessions
          </h1>
          <p className="text-sm text-muted">
            Manage your profile details, security settings, and active login sessions
          </p>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Profile Details Card */}
          <div className="bg-paper border border-rule rounded-surface p-6 shadow-sm">
            <h2 className="font-title text-base font-semibold text-ink flex items-center gap-2 mb-4 border-b border-rule pb-2">
              <User className="w-5 h-5 text-accent" /> Profile Details
            </h2>

            {profileSuccess && (
              <div className="flex items-start gap-3 rounded-control border border-ok-edge bg-ok-wash p-4 text-xs text-ok mb-4">
                <CheckCircle className="w-4 h-4 flex-shrink-0 mt-0.5" />
                <div>{profileSuccess}</div>
              </div>
            )}

            {profileError && (
              <div className="flex items-start gap-3 rounded-control border border-danger-edge bg-danger-wash p-4 text-xs text-danger mb-4">
                <AlertTriangle className="w-4 h-4 flex-shrink-0 mt-0.5" />
                <div>{profileError}</div>
              </div>
            )}

            <form
              onSubmit={(e) => {
                e.preventDefault();
                updateProfileMutation.mutate();
              }}
              className="space-y-4"
            >
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                    First Name
                  </label>
                  <input
                    type="text"
                    value={firstName}
                    onChange={(e) => setFirstName(e.target.value)}
                    className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus focus:ring-1 focus:ring-focus font-sans"
                  />
                </div>
                <div>
                  <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                    Last Name
                  </label>
                  <input
                    type="text"
                    value={lastName}
                    onChange={(e) => setLastName(e.target.value)}
                    className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus focus:ring-1 focus:ring-focus font-sans"
                  />
                </div>
              </div>

              <div>
                <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                  Display Name
                </label>
                <input
                  type="text"
                  value={displayName}
                  onChange={(e) => setDisplayName(e.target.value)}
                  className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus focus:ring-1 focus:ring-focus font-sans"
                  placeholder="e.g. John Doe"
                />
              </div>

              <div>
                <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                  Organization / Company
                </label>
                <input
                  type="text"
                  value={organization}
                  onChange={(e) => setOrganization(e.target.value)}
                  className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus focus:ring-1 focus:ring-focus font-sans"
                  placeholder="e.g. Acme Corp"
                />
              </div>

              <div className="pt-2">
                <button
                  type="submit"
                  disabled={updateProfileMutation.isPending}
                  className="px-4 py-2 bg-accent hover:bg-accent-deep text-accent-ink text-sm font-semibold rounded-control transition duration-short disabled:opacity-50"
                >
                  {updateProfileMutation.isPending ? 'Saving...' : 'Save Profile'}
                </button>
              </div>
            </form>
          </div>

          {/* Change Password Card */}
          <div className="bg-paper border border-rule rounded-surface p-6 shadow-sm">
            <h2 className="font-title text-base font-semibold text-ink flex items-center gap-2 mb-4 border-b border-rule pb-2">
              <KeyRound className="w-5 h-5 text-accent" /> Change Password
            </h2>

            {passwordSuccess && (
              <div className="flex items-start gap-3 rounded-control border border-ok-edge bg-ok-wash p-4 text-xs text-ok mb-4">
                <CheckCircle className="w-4 h-4 flex-shrink-0 mt-0.5" />
                <div>{passwordSuccess}</div>
              </div>
            )}

            {passwordError && (
              <div className="flex items-start gap-3 rounded-control border border-danger-edge bg-danger-wash p-4 text-xs text-danger mb-4">
                <AlertTriangle className="w-4 h-4 flex-shrink-0 mt-0.5" />
                <div>{passwordError}</div>
              </div>
            )}

            <form
              onSubmit={(e) => {
                e.preventDefault();
                changePasswordMutation.mutate();
              }}
              className="space-y-4"
            >
              <div>
                <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                  Current Password
                </label>
                <input
                  type="password"
                  value={currentPassword}
                  onChange={(e) => setCurrentPassword(e.target.value)}
                  required
                  className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus focus:ring-1 focus:ring-focus font-sans"
                />
              </div>

              <div>
                <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                  New Password
                </label>
                <input
                  type="password"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  required
                  className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus focus:ring-1 focus:ring-focus font-sans"
                />
              </div>

              <div>
                <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                  Confirm New Password
                </label>
                <input
                  type="password"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  required
                  className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus focus:ring-1 focus:ring-focus font-sans"
                />
              </div>

              <div className="pt-2">
                <button
                  type="submit"
                  disabled={changePasswordMutation.isPending}
                  className="px-4 py-2 bg-accent hover:bg-accent-deep text-accent-ink text-sm font-semibold rounded-control transition duration-short disabled:opacity-50"
                >
                  {changePasswordMutation.isPending ? 'Updating...' : 'Update Password'}
                </button>
              </div>
            </form>
          </div>
        </div>

        {/* Sessions Card */}
        <div className="bg-paper border border-rule rounded-surface p-6 shadow-sm">
          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-4 border-b border-rule pb-2">
            <h2 className="font-title text-base font-semibold text-ink flex items-center gap-2">
              <Monitor className="w-5 h-5 text-accent" /> Active Login Sessions
            </h2>
            <button
              onClick={() => {
                if (confirm('Are you sure you want to revoke all other active sessions? You will remain logged in on this browser.')) {
                  revokeAllSessionsMutation.mutate();
                }
              }}
              disabled={revokeAllSessionsMutation.isPending || !sessions || sessions.length <= 1}
              className="text-xs font-semibold text-danger hover:underline disabled:opacity-50 disabled:no-underline"
            >
              Revoke All Other Sessions
            </button>
          </div>

          {isLoadingSessions ? (
            <div className="flex justify-center p-8">
              <div className="w-6 h-6 border-2 border-rule border-t-accent rounded-full animate-spin"></div>
            </div>
          ) : !sessions || sessions.length === 0 ? (
            <p className="text-sm text-muted text-center py-6">No active sessions found.</p>
          ) : (
            <div className="divide-y divide-rule border border-rule rounded-surface overflow-hidden">
              {sessions.map((session) => (
                <div key={session.id} className="p-4 flex flex-col sm:flex-row sm:items-center justify-between gap-4 hover:bg-paper-2 transition">
                  <div className="flex items-start gap-3">
                    <div className="w-8 h-8 rounded-control bg-paper-3 flex items-center justify-center text-muted mt-0.5">
                      <Monitor className="w-4 h-4" />
                    </div>
                    <div>
                      <div className="text-sm font-semibold text-ink flex items-center gap-2">
                        {session.os || 'Unknown OS'} • {session.browser || 'Unknown Browser'}
                        {/* Tag current session (just checking if it's the first login? Or we can't fully know but it is fine to render sessions) */}
                      </div>
                      <div className="text-xs text-muted flex flex-wrap gap-x-4 gap-y-1 mt-0.5">
                        <span className="flex items-center gap-1">
                          <Globe className="w-3.5 h-3.5" /> {session.ipAddress || 'Unknown IP'} {session.location && `(${session.location})`}
                        </span>
                        <span className="flex items-center gap-1">
                          <Clock className="w-3.5 h-3.5" /> Logged in: {new Date(session.loginAt).toLocaleString()}
                        </span>
                      </div>
                    </div>
                  </div>

                  <button
                    onClick={() => {
                      if (confirm('Revoke this session? The device will be signed out immediately.')) {
                        revokeSessionMutation.mutate(session.id);
                      }
                    }}
                    disabled={revokeSessionMutation.isPending}
                    className="self-end sm:self-center p-2 rounded-control text-muted hover:text-danger hover:bg-danger-wash border border-transparent hover:border-danger-edge transition"
                    title="Revoke session"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </NavigationShell>
  );
}
