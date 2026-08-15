/* Hallmark · genre: modern-minimal · macrostructure: Workbench
 * design-system: design.md · designed-as-app
 */
'use client';

import React, { useState } from 'react';
import {
  useQuery,
  useMutation,
  useQueryClient,
  type UseMutationResult,
} from '@tanstack/react-query';
import { api } from '@/lib/api';
import { useAuth } from '@/app/providers';
import NavigationShell from '@/components/NavigationShell';
import {
  Button,
  Field,
  PageHeader,
  SectionCard,
  EmptyState,
  SkeletonLine,
  Loading,
  useToast,
  useConfirm,
} from '@/components/ui';
import { Monitor, Globe, Clock, Trash2 } from 'lucide-react';

interface AccountProfile {
  id: string;
  email: string;
  username?: string;
  firstName?: string;
  lastName?: string;
  displayName?: string;
  organization?: string;
  avatar?: string;
}

interface Session {
  id: string;
  deviceInfo?: string;
  browser?: string;
  os?: string;
  ipAddress?: string;
  location?: string;
  loginAt: string;
}

function messageOf(err: unknown, fallback: string) {
  return err instanceof Error && err.message ? err.message : fallback;
}

export default function ProfilePage() {
  const queryClient = useQueryClient();
  const { refreshUser } = useAuth();
  const { toast, error: toastError } = useToast();
  const confirm = useConfirm();

  const [firstName, setFirstName] = useState('');
  const [lastName, setLastName] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [organization, setOrganization] = useState('');

  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  // ตรวจรหัสผ่านหลังผู้ใช้ออกจากช่องครั้งแรกเท่านั้น ไม่ตรวจทุกตัวอักษร
  const [confirmTouched, setConfirmTouched] = useState(false);

  // PUT /account/profile เขียนทับทุก field ที่มันรู้จัก ไม่ใช่ patch เฉพาะที่ส่งไป
  // จึงต้องโหลดโปรไฟล์เต็มมาก่อน แล้วส่งกลับให้ครบ ไม่งั้น username/avatar/organization
  // จะถูกล้างเป็นค่าว่างทุกครั้งที่กดบันทึก
  const { data: profile, isLoading: isLoadingProfile } = useQuery<AccountProfile>({
    queryKey: ['account-profile'],
    queryFn: async () => (await api.get('/account/profile')).data,
  });

  // ปรับ state ระหว่าง render ตามแนวทางของ React ไม่ใช่ใน effect (กัน cascading render)
  const [syncedProfileId, setSyncedProfileId] = useState<string | undefined>(undefined);
  if (profile && profile.id !== syncedProfileId) {
    setSyncedProfileId(profile.id);
    setFirstName(profile.firstName || '');
    setLastName(profile.lastName || '');
    setDisplayName(profile.displayName || '');
    setOrganization(profile.organization || '');
  }

  const { data: sessions, isLoading: isLoadingSessions } = useQuery<Session[]>({
    queryKey: ['sessions'],
    queryFn: async () => {
      const res = await api.get('/account/sessions');
      return res.data.items || [];
    },
  });

  const saveProfile: UseMutationResult<unknown, Error, void> = useMutation({
    mutationFn: async () => {
      const res = await api.put('/account/profile', {
        firstName,
        lastName,
        displayName,
        organization,
        // ส่งคืนตามที่ได้มา สองอันนี้หน้านี้ไม่ได้แก้ แต่ถ้าไม่ส่งจะโดนล้าง
        username: profile?.username ?? '',
        avatar: profile?.avatar ?? '',
      });
      return res.data;
    },
    // สำเร็จแล้วชื่อในแถบข้างเปลี่ยนให้เห็นเอง จึงไม่ต้อง toast
    onSuccess: async () => {
      queryClient.invalidateQueries({ queryKey: ['account-profile'] });
      await refreshUser();
    },
    onError: (err) =>
      toastError(messageOf(err, 'Could not save your profile'), () => saveProfile.mutate()),
  });

  const changePassword: UseMutationResult<unknown, Error, void> = useMutation({
    mutationFn: async () => {
      const res = await api.post('/account/change-password', {
        currentPassword,
        newPassword,
      });
      return res.data;
    },
    // ผลมองไม่เห็นบนหน้าจอ จึงต้องบอก
    onSuccess: () => {
      toast({ tone: 'ok', message: 'Password changed' });
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
      setConfirmTouched(false);
    },
    onError: (err) => toastError(messageOf(err, 'Could not change the password')),
  });

  const revokeOne: UseMutationResult<void, Error, string> = useMutation({
    mutationFn: async (sessionId: string) => {
      await api.delete(`/account/sessions/${sessionId}`);
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['sessions'] }),
    onError: (err, id) =>
      toastError(messageOf(err, 'Could not revoke that device'), () => revokeOne.mutate(id)),
  });

  const revokeAll: UseMutationResult<void, Error, void> = useMutation({
    mutationFn: async () => {
      await api.delete('/account/sessions');
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['sessions'] }),
    onError: (err) =>
      toastError(messageOf(err, 'Could not revoke the other devices'), () => revokeAll.mutate()),
  });

  const passwordMismatch =
    confirmTouched && confirmPassword.length > 0 && newPassword !== confirmPassword;

  async function askRevokeOne(session: Session) {
    const ok = await confirm({
      title: 'Revoke this device',
      description: `${session.os || 'Unknown OS'} · ${session.browser || 'Unknown browser'}`,
      consequences: [
        'That device is signed out immediately',
        'It has to sign in again to keep working',
      ],
      confirmLabel: 'Revoke',
      danger: true,
    });
    if (ok) revokeOne.mutate(session.id);
  }

  async function askRevokeAll() {
    const ok = await confirm({
      title: 'Revoke every other device',
      consequences: [
        'This browser stays signed in',
        'Every other device is signed out immediately',
      ],
      confirmLabel: 'Revoke all',
      danger: true,
    });
    if (ok) revokeAll.mutate();
  }

  return (
    <NavigationShell>
      <PageHeader
        title="Profile & Sessions"
        description="Your details, your password, and the devices signed in right now"
      />

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <SectionCard title="Your details" description="The display name is what shows in the sidebar">
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              saveProfile.mutate();
            }}
          >
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field
                label="First name"
                value={firstName}
                onChange={(e) => setFirstName(e.target.value)}
                autoComplete="given-name"
              />
              <Field
                label="Last name"
                value={lastName}
                onChange={(e) => setLastName(e.target.value)}
                autoComplete="family-name"
              />
            </div>

            <Field
              label="Display name"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder="Somchai Jaidee"
              helper="Left empty, we use the part of your email before the @"
              autoComplete="nickname"
            />

            <Field
              label="Organisation"
              value={organization}
              onChange={(e) => setOrganization(e.target.value)}
              placeholder="Company or team"
              autoComplete="organization"
            />

            <Button
              type="submit"
              variant="primary"
              loading={saveProfile.isPending}
              disabled={isLoadingProfile}
            >
              Save
            </Button>
          </form>
        </SectionCard>

        <SectionCard title="Change password" description="Other devices stay signed in until you revoke them">
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              setConfirmTouched(true);
              if (newPassword !== confirmPassword) return;
              changePassword.mutate();
            }}
          >
            <Field
              label="Current password"
              type="password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              required
              autoComplete="current-password"
            />
            <Field
              label="New password"
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              required
              autoComplete="new-password"
            />
            <Field
              label="Confirm new password"
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              onBlur={() => setConfirmTouched(true)}
              error={passwordMismatch ? 'The two passwords do not match. Type them again.' : undefined}
              required
              autoComplete="new-password"
            />

            <Button
              type="submit"
              variant="primary"
              loading={changePassword.isPending}
              disabled={passwordMismatch}
            >
              Change password
            </Button>
          </form>
        </SectionCard>
      </div>

      <SectionCard
        className="mt-6"
        title="Active sessions"
        description="See something you do not recognise? Revoke it, then change your password"
        flush
        actions={
          <Button
            size="sm"
            variant="danger"
            onClick={askRevokeAll}
            loading={revokeAll.isPending}
            disabled={!sessions || sessions.length <= 1}
          >
            Revoke other devices
          </Button>
        }
      >
        {isLoadingSessions && (
          <Loading label="Loading sessions">
            <div className="space-y-3 p-4">
              {[0, 1].map((i) => (
                <div key={i}>
                  <SkeletonLine width="45%" className="mb-2 h-4" />
                  <SkeletonLine width="70%" />
                </div>
              ))}
            </div>
          </Loading>
        )}

        {!isLoadingSessions && (!sessions || sessions.length === 0) && (
          <EmptyState
            icon={<Monitor className="h-5 w-5" />}
            title="No active sessions"
            description="Sessions appear here once you sign in from another browser or device"
          />
        )}

        {!isLoadingSessions && sessions && sessions.length > 0 && (
          <ul className="ui-fade divide-y divide-rule">
            {sessions.map((session) => (
              <li
                key={session.id}
                className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between"
              >
                <div className="flex min-w-0 items-start gap-3">
                  <span
                    aria-hidden="true"
                    className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-control bg-paper-3 text-muted"
                  >
                    <Monitor className="h-4 w-4" />
                  </span>
                  <div className="min-w-0">
                    <p className="text-sm font-medium text-ink">
                      {session.os || 'Unknown OS'} ·{' '}
                      {session.browser || 'Unknown browser'}
                    </p>
                    <p className="mt-0.5 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted">
                      <span className="inline-flex items-center gap-1">
                        <Globe className="h-3.5 w-3.5" aria-hidden="true" />
                        {session.ipAddress || 'Unknown IP'}
                        {session.location && ` (${session.location})`}
                      </span>
                      <span className="inline-flex items-center gap-1">
                        <Clock className="h-3.5 w-3.5" aria-hidden="true" />
                        Signed in {new Date(session.loginAt).toLocaleString()}
                      </span>
                    </p>
                  </div>
                </div>

                <Button
                  size="sm"
                  variant="ghost-danger"
                  className="self-end sm:self-center"
                  onClick={() => askRevokeOne(session)}
                  loading={revokeOne.isPending && revokeOne.variables === session.id}
                  aria-label="Revoke this device"
                  icon={<Trash2 className="h-4 w-4" />}
                />
              </li>
            ))}
          </ul>
        )}
      </SectionCard>
    </NavigationShell>
  );
}
