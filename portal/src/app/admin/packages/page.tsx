'use client';

import React, { useRef, useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, fetchAll, type UserSession } from '@/lib/api';
import { useInfiniteList } from '@/lib/useInfiniteList';
import { useDebounced } from '@/lib/useDebounced';
import { useAuth } from '@/app/providers';
import NavigationShell from '@/components/NavigationShell';
import {
  PageHeader,
  EmptyState,
  SkeletonLine,
  Loading,
  useToast,
  useConfirm,
  SearchableSelect,
  Select,
  InfiniteScrollSentinel,
} from '@/components/ui';
import {
  Users,
  Plus,
  Trash2,
  Edit,
  AlertTriangle,
  Layers,
  UserCheck,
  Server,
  Search,
  X,
} from 'lucide-react';

interface RateLimitTier {
  limit: number;
  windowSec: number;
}

interface Package {
  id: string;
  name: string;
  alias: string;
  description?: string;
  enabled?: boolean;
  policies?: {
    rateLimit?: {
      enabled: boolean;
      tiers?: RateLimitTier[];
    };
  };
  properties?: Record<string, unknown> | null;
  userCount?: number;
  serviceCount?: number;
}

interface PackagePath {
  path: string;
  methods: string[];
  policies?: {
    rateLimit?: {
      enabled: boolean;
      tiers?: RateLimitTier[];
    };
  };
}

/** `_id` is the raw Mongo key, kept because some link rows still carry that shape. */
interface ServiceOption {
  id: string;
  _id?: string;
  name: string;
  basePath: string;
  type?: string;
}

interface PackageSvcLink {
  id: string;
  packageId?: string;
  serviceId?: string;
  service?: {
    id: string;
    name: string;
    description?: string;
    type?: string;
    basePath?: string;
  };
  paths?: PackagePath[];
}

// ต้องมี field ที่หน้านี้ไม่ได้แก้ด้วย เพราะ PUT /users เป็น replace ทั้งก้อน
// ไม่ส่งไปเท่ากับสั่งให้ล้างทิ้ง
interface User {
  id: string;
  email: string;
  username?: string;
  firstName?: string;
  lastName?: string;
  displayName?: string;
  organization?: string;
  avatar?: string;
  role: 'user' | 'admin' | 'root';
  packageId?: string;
  verified?: boolean;
  enabled?: boolean;
  expiredAt?: string | null;
  properties?: Record<string, unknown> | null;
}

/** "5/10s", or "none". Window stays in raw seconds, no unit rounding. */
function formatLimit(tiers: RateLimitTier[]): string {
  const first = tiers[0];
  if (!first) return 'none';
  return `${first.limit}/${first.windowSec}s`;
}

/* Mirror canUpdateTarget / canDeleteUser in control-plane/handler/user_handler.go — the API
 * decides, these just hide buttons it would refuse. Keep in sync. What you may do to your own
 * account is stricter here than in the API, so the form cannot lock you out of the portal. */

/** a root edits itself but no other root; an admin edits itself and plain users, never another admin */
function canEditUser(actor: UserSession | null, target: User): boolean {
  if (!actor) return false;
  if (target.role === 'root') return actor.role === 'root' && target.id === actor.id;
  if (actor.role === 'root') return true;
  if (actor.role === 'admin') return target.role === 'user' || target.id === actor.id;
  return false;
}

/** nobody deletes themselves and nobody deletes a root, not even another root; an admin only deletes plain users */
function canDeleteUser(actor: UserSession | null, target: User): boolean {
  if (!actor) return false;
  if (target.id === actor.id) return false;
  if (target.role === 'root') return false;
  if (actor.role === 'root') return true;
  if (actor.role === 'admin') return target.role === 'user';
  return false;
}

export default function AdminPackagesPage() {
  const { user: currentUser } = useAuth();
  const queryClient = useQueryClient();
  const { toast, error: toastError } = useToast();
  const confirm = useConfirm();
  const [activeTab, setActiveTab] = useState<'packages' | 'users'>('packages');

  // Search & filter states
  const [packageSearch, setPackageSearch] = useState('');
  const [userSearch, setUserSearch] = useState('');
  // Both searches run server-side now — a client filter would only see the loaded page.
  const debouncedPackageSearch = useDebounced(packageSearch.trim());
  const debouncedUserSearch = useDebounced(userSearch.trim());
  const packagesScrollRef = useRef<HTMLDivElement>(null);
  const usersScrollRef = useRef<HTMLDivElement>(null);
  const [isNewUserOpen, setIsNewUserOpen] = useState(false);
  const [newEmail, setNewEmail] = useState('');
  const [newFirstName, setNewFirstName] = useState('');
  const [newLastName, setNewLastName] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [newRole, setNewRole] = useState<'user' | 'admin'>('user');
  const [newPackageId, setNewPackageId] = useState('');
  const [newUserError, setNewUserError] = useState<string | null>(null);

  // Form states for Package CRUD
  const [editingPackage, setEditingPackage] = useState<Package | null>(null);
  const [isPackageFormOpen, setIsPackageFormOpen] = useState(false);
  const [packageName, setPackageName] = useState('');
  const [packageAlias, setPackageAlias] = useState('');
  const [packageDescription, setPackageDescription] = useState('');
  const [rateLimitEnabled, setRateLimitEnabled] = useState(true);
  const [tiers, setTiers] = useState<RateLimitTier[]>([{ limit: 10, windowSec: 60 }]);
  
  // Package Services Link Modal States
  const [linkingPackage, setLinkingPackage] = useState<Package | null>(null);
  const [isLinkModalOpen, setIsLinkModalOpen] = useState(false);
  const [selectedServiceId, setSelectedServiceId] = useState('');
  const [linkPath, setLinkPath] = useState('/*');
  const linkMethods = ['GET'];
  const [linkRateLimitEnabled, setLinkRateLimitEnabled] = useState(false);
  const [linkTiers, setLinkTiers] = useState<RateLimitTier[]>([{ limit: 5, windowSec: 10 }]);

  // Users Admin states
  const [editingUser, setEditingUser] = useState<User | null>(null);
  const [isUserModalOpen, setIsUserModalOpen] = useState(false);
  const [userRole, setUserRole] = useState<'user' | 'admin' | 'root'>('user');
  const [userPackageId, setUserPackageId] = useState('');
  const [userVerified, setUserVerified] = useState(false);
  const [userEnabled, setUserEnabled] = useState(true);

  const [formError, setFormError] = useState<string | null>(null);

  // Queries
  const {
    items: packages,
    isLoading: isLoadingPackages,
    hasNextPage: hasMorePackages,
    isFetchingNextPage: isFetchingMorePackages,
    fetchNextPage: fetchMorePackages,
  } = useInfiniteList<Package>(
    ['admin-packages'],
    '/packages',
    debouncedPackageSearch ? { search: debouncedPackageSearch } : undefined
  );

  // The Users tab resolves a user's packageId against this and offers it in a dropdown,
  // so it needs every package — the paged list above is scrolled and search-filtered.
  const { data: packageOptions } = useQuery<Package[]>({
    // Nested under 'admin-packages' so the existing invalidations refresh it too.
    queryKey: ['admin-packages', 'options'],
    queryFn: () => fetchAll<Package>('/packages'),
    enabled: activeTab === 'users',
  });

  // Every service has to be offered in the link picker, so this one reads all pages.
  const { data: services } = useQuery<ServiceOption[]>({
    queryKey: ['admin-services-list'],
    queryFn: () => fetchAll<ServiceOption>('/services'),
  });

  // Which package a self-registered user gets. An account created here is given
  // the same one unless told otherwise, so both routes in end up alike.
  const { data: defaultPackageAlias } = useQuery<string>({
    queryKey: ['register-default-package'],
    queryFn: async () => {
      const res = await api.get('/configuration');
      return res.data?.register?.defaultPackageAlias || '';
    },
    enabled: activeTab === 'users',
  });

  const {
    items: users,
    isLoading: isLoadingUsers,
    hasNextPage: hasMoreUsers,
    isFetchingNextPage: isFetchingMoreUsers,
    fetchNextPage: fetchMoreUsers,
  } = useInfiniteList<User>(
    ['admin-users'],
    '/users',
    debouncedUserSearch ? { search: debouncedUserSearch } : undefined,
    { enabled: activeTab === 'users' }
  );

  // Query Package Service links — drives the checkbox state, so it needs every link.
  const { data: activeLinks, refetch: refetchLinks } = useQuery<PackageSvcLink[]>({
    queryKey: ['package-links', linkingPackage?.id],
    queryFn: async () => {
      if (!linkingPackage) return [];
      return fetchAll<PackageSvcLink>(`/packages/${linkingPackage.id}/services`);
    },
    enabled: !!linkingPackage && isLinkModalOpen,
  });

  /** เปิดฟอร์มพร้อมเติมค่า — ทำตรงนี้ทีเดียว ไม่ต้องมี effect คอยตามซิงก์ */
  function openPackageForm(pkg: Package | null) {
    setEditingPackage(pkg);
    setPackageName(pkg?.name || '');
    setPackageAlias(pkg?.alias || '');
    setPackageDescription(pkg?.description || '');
    setRateLimitEnabled(pkg ? pkg.policies?.rateLimit?.enabled !== false : true);
    setTiers(pkg?.policies?.rateLimit?.tiers || [{ limit: 10, windowSec: 60 }]);
    setFormError(null);
    setIsPackageFormOpen(true);
  }

  function closePackageForm() {
    setIsPackageFormOpen(false);
    setEditingPackage(null);
    setFormError(null);
  }

  function openUserForm(u: User) {
    setEditingUser(u);
    setUserRole(u.role || 'user');
    setUserPackageId(u.packageId || '');
    setUserVerified(u.verified === true);
    setUserEnabled(u.enabled !== false);
    setIsUserModalOpen(true);
  }

  // Mutations
  const savePackageMutation = useMutation({
    mutationFn: async () => {
      setFormError(null);
      const payload = {
        name: packageName,
        alias: packageAlias,
        description: packageDescription,
        enabled: true,
        properties: editingPackage?.properties ?? null,
        policies: {
          rateLimit: {
            enabled: rateLimitEnabled,
            tiers,
          },
        },
      };

      if (editingPackage) {
        await api.put(`/packages/${editingPackage.id}`, payload);
      } else {
        await api.post('/packages', payload);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-packages'] });
      setIsPackageFormOpen(false);
      setEditingPackage(null);
      toast({ tone: 'ok', message: 'Package saved' });
    },
    onError: (err: unknown) => {
      setFormError(err instanceof Error && err.message ? err.message : 'Failed to save package.');
    },
  });

  const deletePackageMutation = useMutation({
    mutationFn: async (id: string) => {
      await api.delete(`/packages/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-packages'] });
      toast({ tone: 'ok', message: 'Package deleted' });
    },
  });

  // Package Service Links Mutations
  const linkServiceMutation = useMutation({
    mutationFn: async () => {
      // throw, not return — a bare return runs onSuccess and clears the form
      // as if the link had been made
      if (!linkingPackage) throw new Error('No package is open to link against');
      if (!selectedServiceId) throw new Error('Choose a service to grant access to');

      const payload = {
        serviceId: selectedServiceId,
        paths: [
          {
            path: linkPath,
            methods: linkMethods,
            policies: {
              rateLimit: {
                enabled: linkRateLimitEnabled,
                tiers: linkTiers,
              },
            },
          },
        ],
      };
      await api.post(`/packages/${linkingPackage.id}/services`, payload);
    },
    onSuccess: () => {
      refetchLinks();
      queryClient.invalidateQueries({ queryKey: ['admin-packages'] });
      setSelectedServiceId('');
      setLinkPath('/*');
      setLinkRateLimitEnabled(false);
    },
    onError: (err: unknown) =>
      toastError(
        err instanceof Error && err.message ? err.message : 'Could not grant access to that service',
      ),
  });

  const unlinkServiceMutation = useMutation({
    mutationFn: async (serviceId: string) => {
      if (!linkingPackage) return;
      await api.delete(`/packages/${linkingPackage.id}/services/${serviceId}`);
    },
    onSuccess: () => {
      refetchLinks();
      queryClient.invalidateQueries({ queryKey: ['admin-packages'] });
    },
  });

  // User Save Mutation
  const saveUserMutation = useMutation({
    mutationFn: async () => {
      if (!editingUser) return;
      // ส่งทั้งก้อนที่อ่านมา แล้วทับเฉพาะที่ฟอร์มนี้แก้
      const payload = {
        username: editingUser.username || '',
        firstName: editingUser.firstName || '',
        lastName: editingUser.lastName || '',
        displayName: editingUser.displayName || '',
        organization: editingUser.organization || '',
        avatar: editingUser.avatar || '',
        expiredAt: editingUser.expiredAt ?? null,
        properties: editingUser.properties ?? null,
        role: userRole,
        packageId: userPackageId || undefined,
        verified: userVerified,
        enabled: userEnabled,
      };
      await api.put(`/users/${editingUser.id}`, payload);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-users'] });
      setIsUserModalOpen(false);
      setEditingUser(null);
      toast({ tone: 'ok', message: 'User updated' });
    },
    onError: (err: unknown) => {
      setFormError(err instanceof Error && err.message ? err.message : 'Failed to update user.');
    },
  });

  const createUserMutation = useMutation({
    mutationFn: async () => {
      setNewUserError(null);
      await api.post('/users', {
        email: newEmail.trim(),
        firstName: newFirstName.trim(),
        lastName: newLastName.trim(),
        password: newPassword,
        role: newRole,
        packageId: newPackageId || undefined,
        // A default install has no mail server, so the account is usable at once
        // and the administrator hands over the password.
        verified: true,
        enabled: true,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-users'] });
      setIsNewUserOpen(false);
      setNewEmail('');
      setNewFirstName('');
      setNewLastName('');
      setNewPassword('');
      setNewRole('user');
      setNewPackageId('');
    },
    onError: (err: unknown) => {
      setNewUserError(err instanceof Error && err.message ? err.message : 'Could not create the account.');
    },
  });

  const deleteUserMutation = useMutation({
    mutationFn: async (id: string) => {
      await api.delete(`/users/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-users'] });
      toast({ tone: 'ok', message: 'Account deleted' });
    },
    onError: (err: unknown) =>
      toastError(err instanceof Error && err.message ? err.message : 'Could not delete the account'),
  });

  async function askDeletePackage(pkg: Package) {
    const ok = await confirm({
      title: `Delete package "${pkg.name}"`,
      consequences: [
        'This cannot be undone',
        'Everyone on this package loses access to the services it grants',
        'They need moving to another package before they can call the APIs again',
      ],
      typeToConfirm: pkg.name,
      confirmLabel: 'Delete package',
      danger: true,
    });
    if (ok) deletePackageMutation.mutate(pkg.id);
  }

  async function askDeleteUser(u: User) {
    const ok = await confirm({
      title: 'Delete this account for good',
      description: u.email,
      consequences: [
        'This cannot be undone',
        'Every API key on this account stops working immediately',
        'Anything calling with those keys will start failing',
      ],
      typeToConfirm: u.email,
      confirmLabel: 'Delete account',
      danger: true,
    });
    if (ok) deleteUserMutation.mutate(u.id);
  }

  // Rate tier manipulation helpers
  const addTier = (isLink = false) => {
    const setter = isLink ? setLinkTiers : setTiers;
    setter((prev) => [...prev, { limit: 100, windowSec: 3600 }]);
  };

  const removeTier = (index: number, isLink = false) => {
    const setter = isLink ? setLinkTiers : setTiers;
    setter((prev) => prev.filter((_, i) => i !== index));
  };

  const updateTier = (index: number, field: keyof RateLimitTier, value: number, isLink = false) => {
    const setter = isLink ? setLinkTiers : setTiers;
    setter((prev) => prev.map((t, i) => (i === index ? { ...t, [field]: value } : t)));
  };

  // On your own row the form gives up the switches that would lock you out of the portal.
  const editingSelf = !!editingUser && editingUser.id === currentUser?.id;

  // Mirror canSetNewRole: only root hands out a role above user, and the one admin row an admin
  // may leave on admin is its own.
  const editRoleOptions = [
    { value: 'user', label: 'User' },
    ...(currentUser?.role === 'root' || (editingSelf && editingUser?.role === 'admin')
      ? [{ value: 'admin', label: 'Administrator' }]
      : []),
    ...(editingUser?.role === 'root' ? [{ value: 'root', label: 'Root' }] : []),
  ];

  // Mirror canSetUserRole: a new account is a plain user unless root says otherwise.
  const createRoleOptions = [
    { value: 'user', label: 'User' },
    ...(currentUser?.role === 'root' ? [{ value: 'admin', label: 'Administrator' }] : []),
  ];

  return (
    <NavigationShell>
      <PageHeader
        title="Packages & Users"
        description="Rate-limit tiers, and the accounts attached to each of them"
      />

      <div className="space-y-6">
        {/* Toggle tabs */}
        <div className="flex border-b border-rule pb-px gap-6">
          <button
            onClick={() => setActiveTab('packages')}
            className={`pb-3 text-sm font-semibold border-b-2 transition ${
              activeTab === 'packages'
                ? 'border-accent text-ink font-bold'
                : 'border-transparent text-muted hover:text-ink'
            }`}
          >
            Package tiers
          </button>
          <button
            onClick={() => setActiveTab('users')}
            className={`pb-3 text-sm font-semibold border-b-2 transition ${
              activeTab === 'users'
                ? 'border-accent text-ink font-bold'
                : 'border-transparent text-muted hover:text-ink'
            }`}
          >
            Users
          </button>
        </div>

        {/* ================================= PACKAGES TAB ================================= */}
        {activeTab === 'packages' && (
          <div className="space-y-6">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <h2 className="text-xs font-bold text-muted uppercase tracking-wider">Package Management</h2>
              <div className="flex items-center gap-2">
                <div className="relative w-56">
                  <Search className="absolute left-2.5 top-2.5 h-3.5 w-3.5 text-muted pointer-events-none" />
                  <input
                    type="text"
                    value={packageSearch}
                    onChange={(e) => setPackageSearch(e.target.value)}
                    placeholder="Search by name or alias"
                    className="w-full bg-paper-2 border border-rule rounded-control pl-8 pr-3 py-1.5 text-xs text-ink outline-none focus:border-focus"
                  />
                </div>
                <button
                  onClick={() => {
                    setEditingPackage(null);
                    setIsPackageFormOpen(true);
                  }}
                  className="px-3 py-1.5 bg-accent hover:bg-accent-deep text-accent-ink text-xs font-semibold rounded-control transition duration-short flex items-center gap-1 shrink-0"
                >
                  <Plus className="w-4 h-4" /> Create Package
                </button>
              </div>
            </div>

            {/* Package Form Modal */}
            {isPackageFormOpen && (
              <div className="ui-modal-scrim">
                <div className="bg-paper border border-rule rounded-surface p-6 w-full max-w-lg shadow-sm space-y-4 max-h-[90vh] overflow-y-auto">
                  <h3 className="font-title text-base font-bold text-ink">
                    {editingPackage ? 'Edit Package Tier' : 'Create Package Tier'}
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
                      savePackageMutation.mutate();
                    }}
                    className="space-y-4 text-xs"
                  >
                    <div className="grid grid-cols-2 gap-4">
                      <div>
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          Alias Key<span className="ui-field__req" aria-hidden="true">*</span>
                        </label>
                        <input
                          type="text"
                          required
                          value={packageAlias}
                          onChange={(e) => setPackageAlias(e.target.value)}
                          placeholder="e.g. free"
                          disabled={!!editingPackage}
                          className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                        />
                      </div>
                      <div>
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          Display Name<span className="ui-field__req" aria-hidden="true">*</span>
                        </label>
                        <input
                          type="text"
                          required
                          value={packageName}
                          onChange={(e) => setPackageName(e.target.value)}
                          placeholder="e.g. Free Tier"
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
                        value={packageDescription}
                        onChange={(e) => setPackageDescription(e.target.value)}
                        placeholder="Limits to 10 requests per minute"
                        className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                      />
                    </div>

                    {/* Rate Limits Tiers section */}
                    <div className="space-y-3 border-t border-rule pt-3">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                          <input
                            type="checkbox"
                            id="pkg-rate-enabled"
                            checked={rateLimitEnabled}
                            onChange={(e) => setRateLimitEnabled(e.target.checked)}
                            className="w-4 h-4 accent-accent"
                          />
                          <label htmlFor="pkg-rate-enabled" className="text-xs font-semibold text-ink-2 select-none cursor-pointer">
                            Enable Global Rate Limiting
                          </label>
                        </div>
                        {rateLimitEnabled && (
                          <button
                            type="button"
                            onClick={() => addTier(false)}
                            className="text-accent hover:underline font-semibold"
                          >
                            + Add Tier
                          </button>
                        )}
                      </div>

                      {rateLimitEnabled && (
                        <div className="space-y-2">
                          {tiers.map((tier, index) => (
                            <div key={index} className="flex gap-3 items-center">
                              <div className="flex-1">
                                <label className="text-[9px] font-semibold text-muted uppercase tracking-wider block mb-0.5">
                                  Requests Limit
                                </label>
                                <input
                                  type="number"
                                  required
                                  value={tier.limit}
                                  onChange={(e) => updateTier(index, 'limit', parseInt(e.target.value) || 1, false)}
                                  className="w-full bg-paper-2 border border-rule rounded-control px-2.5 py-1 text-xs text-ink outline-none focus:border-focus font-mono"
                                />
                              </div>
                              <div className="flex-1">
                                <label className="text-[9px] font-semibold text-muted uppercase tracking-wider block mb-0.5">
                                  Window Size (Seconds)
                                </label>
                                <input
                                  type="number"
                                  required
                                  value={tier.windowSec}
                                  onChange={(e) => updateTier(index, 'windowSec', parseInt(e.target.value) || 1, false)}
                                  className="w-full bg-paper-2 border border-rule rounded-control px-2.5 py-1 text-xs text-ink outline-none focus:border-focus font-mono"
                                />
                              </div>
                              <button
                                type="button"
                                onClick={() => removeTier(index, false)}
                                disabled={tiers.length <= 1}
                                className="p-1.5 rounded-control text-muted hover:text-danger hover:bg-danger-wash disabled:opacity-30 mt-4"
                              >
                                <Trash2 className="w-4 h-4" />
                              </button>
                            </div>
                          ))}
                        </div>
                      )}
                    </div>

                    <div className="flex justify-end gap-3 pt-2">
                      <button
                        type="button"
                        onClick={closePackageForm}
                        className="px-4 py-2 border border-rule hover:border-faint rounded-control text-ink-2 hover:bg-paper-2 transition"
                      >
                        Cancel
                      </button>
                      <button
                        type="submit"
                        disabled={savePackageMutation.isPending}
                        className="px-4 py-2 bg-accent hover:bg-accent-deep text-accent-ink font-semibold rounded-control transition"
                      >
                        {savePackageMutation.isPending ? 'Saving...' : 'Save Package'}
                      </button>
                    </div>
                  </form>
                </div>
              </div>
            )}

            {/* Link Services Modal */}
            {isLinkModalOpen && linkingPackage && (
              <div className="ui-modal-scrim">
                <div className="bg-paper border border-rule rounded-surface p-6 w-full max-w-xl shadow-sm space-y-4 max-h-[90vh] overflow-y-auto">
                  <div className="flex justify-between items-center">
                    <h3 className="font-title text-base font-bold text-ink">
                      Link Services: {linkingPackage.name}
                    </h3>
                    {/* Links apply the moment they are made, so there is nothing to cancel —
                        unlike the form modals, this one only needs a way out */}
                    <button
                      onClick={() => setIsLinkModalOpen(false)}
                      className="p-1.5 rounded-control text-muted transition hover:bg-paper-3 hover:text-ink"
                      aria-label="Close"
                      title="Close"
                    >
                      <X className="h-4 w-4" />
                    </button>
                  </div>

                  {/* Add Service Link form */}
                  <form
                    onSubmit={(e) => {
                      e.preventDefault();
                      linkServiceMutation.mutate();
                    }}
                    className="space-y-4 text-xs"
                  >
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                      <div>
                        {/* SearchableSelect's own label is sentence case — spell it out here
                            instead so it reads the same as the labels beside it */}
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          Select Service<span className="ui-field__req" aria-hidden="true">*</span>
                        </label>
                        <SearchableSelect
                          value={selectedServiceId}
                          onChange={setSelectedServiceId}
                          options={
                            services?.map((svc) => ({
                              value: svc.id,
                              label: svc.name,
                              subtext: svc.basePath,
                            })) || []
                          }
                          placeholder="Choose service..."
                          searchPlaceholder="Search by name or path..."
                        />
                      </div>

                      <div>
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          Path Rule<span className="ui-field__req" aria-hidden="true">*</span>
                        </label>
                        <input
                          type="text"
                          required
                          value={linkPath}
                          onChange={(e) => setLinkPath(e.target.value)}
                          className="w-full bg-paper border border-rule rounded-control px-3 py-2 text-xs text-ink outline-none font-mono focus:border-rule-2"
                        />
                      </div>
                    </div>

                    <div className="flex items-center gap-2">
                      <input
                        type="checkbox"
                        id="link-rate-enabled"
                        checked={linkRateLimitEnabled}
                        onChange={(e) => setLinkRateLimitEnabled(e.target.checked)}
                        className="w-4 h-4 accent-accent"
                      />
                      <label htmlFor="link-rate-enabled" className="text-xs font-semibold text-ink-2 select-none cursor-pointer">
                        Apply Path-specific Rate Limit
                      </label>
                    </div>

                    {linkRateLimitEnabled && (
                      <div className="space-y-3">
                        {linkTiers.map((tier, index) => (
                          <div key={index} className="flex gap-3 items-center">
                            <div className="flex-1">
                              <label className="text-[10px] font-semibold text-muted uppercase tracking-wider block mb-1">Requests</label>
                              <input
                                type="number"
                                required
                                value={tier.limit}
                                onChange={(e) => updateTier(index, 'limit', parseInt(e.target.value) || 1, true)}
                                className="w-full bg-paper border border-rule rounded-control px-2 py-0.5 text-xs font-mono outline-none focus:border-rule-2"
                              />
                            </div>
                            <div className="flex-1">
                              <label className="text-[10px] font-semibold text-muted uppercase tracking-wider block mb-1">Window (Sec)</label>
                              <input
                                type="number"
                                required
                                value={tier.windowSec}
                                onChange={(e) => updateTier(index, 'windowSec', parseInt(e.target.value) || 1, true)}
                                className="w-full bg-paper border border-rule rounded-control px-2 py-0.5 text-xs font-mono outline-none focus:border-rule-2"
                              />
                            </div>
                            <button
                              type="button"
                              onClick={() => removeTier(index, true)}
                              disabled={linkTiers.length <= 1}
                              className="p-1 text-muted hover:text-danger self-end"
                            >
                              <Trash2 className="w-4 h-4" />
                            </button>
                          </div>
                        ))}
                      </div>
                    )}

                    <div className="flex justify-end pt-2">
                      <button
                        type="submit"
                        disabled={linkServiceMutation.isPending || !selectedServiceId}
                        className="px-3 py-1.5 bg-accent hover:bg-accent-deep text-accent-ink font-semibold rounded-control transition"
                      >
                        Grant Service Access
                      </button>
                    </div>
                  </form>

                  {/* Active Links list */}
                  <div className="space-y-2 mt-4">
                    <h4 className="text-xs font-bold text-muted uppercase tracking-wider">Access Granted Services</h4>
                    {activeLinks?.length === 0 ? (
                      <p className="py-4 text-center text-sm text-muted">This package does not grant access to any service yet.</p>
                    ) : (
                      <div className="divide-y divide-rule border border-rule rounded-surface overflow-hidden bg-paper">
                        {activeLinks?.map((link) => {
                          const serviceId = link.service?.id || link.serviceId || '';
                          const svc = link.service || services?.find((s) => s.id === serviceId || s._id === serviceId);
                          const svcName = svc?.name || link.service?.name || 'API Service';
                          const svcBasePath = svc?.basePath || link.service?.basePath || '';
                          const svcType = svc?.type || link.service?.type;
                          // Rule already reads off the first path, so the limit beside it has to as well
                          const rateLimit = link.paths?.[0]?.policies?.rateLimit;
                          const tiers = rateLimit?.enabled ? rateLimit.tiers ?? [] : [];

                          return (
                            <div key={link.id} className="p-3 flex items-center justify-between gap-4 text-xs hover:bg-paper-2 transition">
                              <div className="flex items-center gap-2.5">
                                <div className="p-1.5 rounded-control bg-paper-2 text-accent border border-rule">
                                  <Server className="w-4 h-4" />
                                </div>
                                <div>
                                  <div className="flex items-center gap-2">
                                    <p className="font-semibold text-ink">{svcName}</p>
                                    {svcType && (
                                      <span className="text-[9px] uppercase font-bold px-1.5 py-0.2 rounded-chip bg-accent-wash border border-accent-edge text-accent">
                                        {svcType}
                                      </span>
                                    )}
                                  </div>
                                  <div className="flex flex-wrap gap-x-2 gap-y-0.5 text-[10px] text-muted font-mono mt-0.5">
                                    {svcBasePath && <span>Base: {svcBasePath}</span>}
                                    <span>Rule: {link.paths?.[0]?.path || '/*'}</span>
                                    <span>Limit: {formatLimit(tiers)}</span>
                                  </div>
                                </div>
                              </div>
                              <button
                                onClick={() => unlinkServiceMutation.mutate(serviceId)}
                                className="px-2 py-1 text-xs font-semibold text-danger hover:bg-danger-wash rounded-control border border-transparent hover:border-danger-edge transition"
                              >
                                Revoke
                              </button>
                            </div>
                          );
                        })}
                      </div>
                    )}
                  </div>
                </div>
              </div>
            )}

            {isLoadingPackages ? (
              <Loading label="Loading packages">
                <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                  {[0, 1].map((i) => (
                    <div key={i} className="ui-card space-y-2 p-4">
                      <SkeletonLine width="45%" className="h-4" />
                      <SkeletonLine width="70%" />
                    </div>
                  ))}
                </div>
              </Loading>
            ) : packages.length === 0 ? (
              <div className="ui-card">
                <EmptyState
                  icon={<Layers className="h-5 w-5" />}
                  title={debouncedPackageSearch ? 'No packages match that search' : 'No packages yet'}
                  description={
                    debouncedPackageSearch
                      ? `Nothing matches "${debouncedPackageSearch}". Try part of a name or alias.`
                      : 'Create one to set how many calls a user gets per window.'
                  }
                />
              </div>
            ) : (
              <div
                ref={packagesScrollRef}
                className="max-h-[min(944px,70vh)] overflow-y-auto"
              >
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {packages.map((pkg) => (
                  <div key={pkg.id} className="bg-paper border border-rule rounded-surface p-4 shadow-sm hover:border-faint transition flex flex-col justify-between h-44">
                    <div>
                      <div className="flex items-center justify-between gap-2">
                        <span className="font-title text-sm font-semibold text-ink">{pkg.name}</span>
                        <span className="text-[10px] font-mono px-2 py-0.5 rounded-chip bg-accent-wash border border-accent-edge text-accent">
                          Alias: {pkg.alias}
                        </span>
                      </div>
                      <p className="text-xs text-muted mt-2 line-clamp-2">{pkg.description || 'No description.'}</p>
                      
                      <div className="mt-3 text-[10px] text-muted flex gap-4">
                        <span className="flex items-center gap-1">
                          <Users className="w-3.5 h-3.5" /> Users: {pkg.userCount || 0}
                        </span>
                        <span className="flex items-center gap-1">
                          <Server className="w-3.5 h-3.5" /> Mapped APIs: {pkg.serviceCount || 0}
                        </span>
                      </div>
                    </div>

                    <div className="flex justify-between items-center border-t border-rule pt-2 mt-2">
                      <button
                        onClick={() => {
                          setLinkingPackage(pkg);
                          setIsLinkModalOpen(true);
                        }}
                        className="px-2.5 py-1 bg-paper border border-rule hover:border-faint text-[10px] font-semibold rounded-chip text-ink-2 hover:bg-paper-2 transition flex items-center gap-1"
                      >
                        <Layers className="w-3.5 h-3.5" /> Mapped Services
                      </button>

                      <div className="flex gap-2">
                        <button
                          onClick={() => openPackageForm(pkg)}
                          className="p-1 rounded-control text-muted hover:text-ink hover:bg-paper-2 transition"
                          title="Edit package"
                        >
                          <Edit className="w-3.5 h-3.5" />
                        </button>
                        <button
                          onClick={() => askDeletePackage(pkg)}
                          className="p-1 rounded-control text-muted hover:text-danger hover:bg-danger-wash transition"
                          title="Delete package"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
              <InfiniteScrollSentinel
                rootRef={packagesScrollRef}
                hasNextPage={hasMorePackages}
                isFetchingNextPage={isFetchingMorePackages}
                fetchNextPage={fetchMorePackages}
              />
              </div>
            )}
          </div>
        )}

        {/* ================================= USERS TAB ================================= */}
        {activeTab === 'users' && (
          <div className="space-y-6">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <h2 className="text-xs font-bold text-muted uppercase tracking-wider">User Account Management</h2>
              <div className="flex items-center gap-2">
                <div className="relative w-56">
                  <Search className="absolute left-2.5 top-2.5 h-3.5 w-3.5 text-muted pointer-events-none" />
                  <input
                    value={userSearch}
                    onChange={(e) => setUserSearch(e.target.value)}
                    placeholder="Search by email or name"
                    className="w-full bg-paper-2 border border-rule rounded-control pl-8 pr-3 py-1.5 text-xs text-ink outline-none focus:border-focus"
                  />
                </div>
                <button
                  type="button"
                  onClick={() => {
                    setNewUserError(null);
                    const fallback = packageOptions?.find((p) => p.alias === defaultPackageAlias);
                    setNewPackageId(fallback?.id || '');
                    setIsNewUserOpen(true);
                  }}
                  className="px-3 py-1.5 bg-accent hover:bg-accent-deep text-accent-ink text-xs font-semibold rounded-control transition duration-short flex items-center gap-1 shrink-0"
                >
                  <Plus className="w-4 h-4" /> Create User
                </button>
              </div>
            </div>

            {isNewUserOpen && (
              <div className="ui-modal-scrim">
                <div className="bg-paper border border-rule rounded-surface p-6 w-full max-w-md shadow-sm space-y-4 max-h-[90vh] overflow-y-auto">
                  <h3 className="font-title text-base font-bold text-ink">New user</h3>
                  <p className="text-xs text-muted">
                    The account works straight away. Hand the password over yourself, and ask them to
                    change it from Profile.
                  </p>

                  {newUserError && (
                    <div className="flex items-start gap-3 rounded-control border border-danger-edge bg-danger-wash p-4 text-xs text-danger">
                      <AlertTriangle className="w-4 h-4 mt-0.5 flex-shrink-0" />
                      <div>{newUserError}</div>
                    </div>
                  )}

                  <form
                    onSubmit={(e) => {
                      e.preventDefault();
                      createUserMutation.mutate();
                    }}
                    className="space-y-3 text-xs"
                  >
                    <div>
                      <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                        Email Address<span className="ui-field__req" aria-hidden="true">*</span>
                      </label>
                      <input
                        required
                        type="email"
                        value={newEmail}
                        onChange={(e) => setNewEmail(e.target.value)}
                        className="ui-input"
                      />
                    </div>
                    <div className="flex gap-2">
                      <div className="flex-1 min-w-0">
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          First Name<span className="ui-field__req" aria-hidden="true">*</span>
                        </label>
                        <input
                          required
                          value={newFirstName}
                          onChange={(e) => setNewFirstName(e.target.value)}
                          className="ui-input"
                        />
                      </div>
                      <div className="flex-1 min-w-0">
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          Last Name<span className="ui-field__req" aria-hidden="true">*</span>
                        </label>
                        <input
                          required
                          value={newLastName}
                          onChange={(e) => setNewLastName(e.target.value)}
                          className="ui-input"
                        />
                      </div>
                    </div>
                    <div>
                      <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                        Password<span className="ui-field__req" aria-hidden="true">*</span>
                      </label>
                      <input
                        required
                        type="text"
                        value={newPassword}
                        onChange={(e) => setNewPassword(e.target.value)}
                        placeholder="Password to hand over"
                        className="ui-input font-mono"
                      />
                    </div>
                    <div className="flex gap-2">
                      <div className="flex-1 min-w-0">
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          System Role
                        </label>
                        <Select
                          value={newRole}
                          onChange={(val) => setNewRole(val as 'user' | 'admin')}
                          options={createRoleOptions}
                        />
                      </div>
                      <div className="flex-1 min-w-0">
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          Assigned Package Tier
                        </label>
                        <Select
                          value={newPackageId}
                          onChange={setNewPackageId}
                          options={[
                            { value: '', label: 'No package (cannot call anything)' },
                            ...(packageOptions || []).map((pkg) => ({
                              value: pkg.id,
                              label: pkg.name,
                              subtext: pkg.alias,
                            })),
                          ]}
                        />
                      </div>
                    </div>

                    <div className="flex justify-end gap-3 pt-1">
                      <button
                        type="button"
                        onClick={() => setIsNewUserOpen(false)}
                        className="px-4 py-2 border border-rule hover:border-faint rounded-control text-ink-2 hover:bg-paper-2 transition"
                      >
                        Cancel
                      </button>
                      <button
                        type="submit"
                        disabled={createUserMutation.isPending}
                        className="px-4 py-2 rounded-control bg-accent text-accent-ink text-xs font-semibold disabled:opacity-50"
                      >
                        {createUserMutation.isPending ? 'Creating...' : 'Create User'}
                      </button>
                    </div>
                  </form>
                </div>
              </div>
            )}

            {/* Edit User Modal */}
            {isUserModalOpen && editingUser && (
              <div className="ui-modal-scrim">
                <div className="bg-paper border border-rule rounded-surface p-6 w-full max-w-md shadow-sm space-y-4 max-h-[90vh] overflow-y-auto">
                  <h3 className="font-title text-base font-bold text-ink">
                    Edit User Profile: {editingUser.email}
                  </h3>

                  {editingSelf && (
                    <p className="text-xs text-muted">
                      {editingUser.role === 'root'
                        ? 'This is your own root account. Only the package tier is yours to change - giving up the role or the sign-in would shut you out of the portal with no way back in.'
                        : 'This is your own account. You can change the package tier or step down to a lower role, nothing else - the rest would shut you out of the portal.'}
                    </p>
                  )}

                  {formError && (
                    <div className="flex items-start gap-3 rounded-control border border-danger-edge bg-danger-wash p-4 text-xs text-danger">
                      <AlertTriangle className="w-4 h-4 mt-0.5 flex-shrink-0" />
                      <div>{formError}</div>
                    </div>
                  )}

                  <form
                    onSubmit={(e) => {
                      e.preventDefault();
                      saveUserMutation.mutate();
                    }}
                    className="space-y-4 text-xs"
                  >
                    <div>
                      <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                        System Role
                      </label>
                      <Select
                        value={userRole}
                        onChange={(val) => setUserRole(val as 'user' | 'admin' | 'root')}
                        disabled={editingUser.role === 'root'} // a root may not demote itself
                        options={editRoleOptions}
                      />
                    </div>

                    <div>
                      <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                        Assigned Package Tier
                      </label>
                      <Select
                        value={userPackageId}
                        onChange={setUserPackageId}
                        options={[
                          { value: '', label: 'No Package Assigned' },
                          ...(packageOptions || []).map((pkg) => ({
                            value: pkg.id,
                            label: pkg.name,
                            subtext: pkg.alias,
                          })),
                        ]}
                      />
                    </div>

                    <div className="flex flex-col gap-2 pt-2 border-t border-rule">
                      <div className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          id="usr-verified"
                          checked={userVerified}
                          onChange={(e) => setUserVerified(e.target.checked)}
                          disabled={editingSelf}
                          className="w-4 h-4 accent-accent"
                        />
                        <label htmlFor="usr-verified" className="text-xs font-semibold text-ink-2 select-none cursor-pointer">
                          Account Verified (Email confirm)
                        </label>
                      </div>

                      <div className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          id="usr-enabled"
                          checked={userEnabled}
                          onChange={(e) => setUserEnabled(e.target.checked)}
                          disabled={editingSelf}
                          className="w-4 h-4 accent-accent"
                        />
                        <label htmlFor="usr-enabled" className="text-xs font-semibold text-ink-2 select-none cursor-pointer">
                          Account Enabled (Can authenticate)
                        </label>
                      </div>
                    </div>

                    <div className="flex justify-end gap-3 pt-2">
                      <button
                        type="button"
                        onClick={() => {
                          setIsUserModalOpen(false);
                          setEditingUser(null);
                        }}
                        className="px-4 py-2 border border-rule hover:border-faint rounded-control text-ink-2 hover:bg-paper-2 transition"
                      >
                        Cancel
                      </button>
                      <button
                        type="submit"
                        disabled={saveUserMutation.isPending}
                        className="px-4 py-2 bg-accent hover:bg-accent-deep text-accent-ink font-semibold rounded-control transition"
                      >
                        {saveUserMutation.isPending ? 'Saving...' : 'Update Details'}
                      </button>
                    </div>
                  </form>
                </div>
              </div>
            )}

            {/* Users Accounts list table */}
            {isLoadingUsers ? (
              <Loading label="Loading users">
                <div className="space-y-2">
                  {[0, 1, 2].map((i) => (
                    <SkeletonLine key={i} className="h-8" />
                  ))}
                </div>
              </Loading>
            ) : users.length === 0 ? (
              <div className="ui-card">
                <EmptyState
                  icon={<Users className="h-5 w-5" />}
                  title={debouncedUserSearch ? 'Nobody matches that search' : 'No users yet'}
                  description={
                    debouncedUserSearch
                      ? `No account matches "${debouncedUserSearch}". Try part of an email address.`
                      : 'Accounts show up here once people sign up, or once you create one.'
                  }
                />
              </div>
            ) : (
              <div className="bg-paper border border-rule rounded-surface overflow-hidden shadow-sm">
                <div
                  ref={usersScrollRef}
                  className="max-h-[min(430px,70vh)] overflow-x-auto overflow-y-auto"
                >
                  <table className="w-full text-left border-collapse text-xs">
                    <thead className="sticky top-0 z-10">
                      <tr className="border-b border-rule text-muted text-[10px] uppercase font-bold tracking-wider bg-paper-2">
                        <th className="py-2 px-3">Email Address</th>
                        <th className="py-2 px-3">Role</th>
                        <th className="py-2 px-3">Package Tier</th>
                        <th className="py-2 px-3">Verified</th>
                        <th className="py-2 px-3">Status</th>
                        <th className="py-2 px-3 text-right">Actions</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-rule font-sans">
                      {users.map((u) => {
                        const pkg = packageOptions?.find((p) => p.id === u.packageId);
                        return (
                          <tr key={u.id} className="hover:bg-paper-2 transition-colors">
                            <td className="py-2.5 px-3 font-semibold text-ink">{u.email}</td>
                            <td className="py-2.5 px-3">
                              <span className="px-2 py-0.5 rounded-chip text-[9px] uppercase font-bold bg-accent-wash border border-accent-edge text-accent">
                                {u.role}
                              </span>
                            </td>
                            <td className="py-2.5 px-3 font-mono text-muted">{pkg?.name || 'None'}</td>
                            <td className="py-2.5 px-3">
                              {u.verified ? (
                                <span className="text-ok font-bold text-[10px] flex items-center gap-1">
                                  <UserCheck className="w-3.5 h-3.5" /> Verified
                                </span>
                              ) : (
                                <span className="text-warn font-semibold text-[10px]">Pending</span>
                              )}
                            </td>
                            <td className="py-2.5 px-3">
                              {u.enabled !== false ? (
                                <span className="text-ok font-semibold">Active</span>
                              ) : (
                                <span className="text-danger font-semibold">Suspended</span>
                              )}
                            </td>
                            <td className="py-2.5 px-3 text-right space-x-1">
                              <button
                                onClick={() => openUserForm(u)}
                                className="p-1.5 rounded-control text-muted hover:text-ink hover:bg-paper-3 transition inline-flex items-center align-middle disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-muted"
                                title={
                                  canEditUser(currentUser, u)
                                    ? 'Edit User'
                                    : u.id === currentUser?.id
                                      ? 'Edit your own account from your profile page'
                                      : `A ${u.role} account is out of your reach`
                                }
                                disabled={!canEditUser(currentUser, u)}
                              >
                                <Edit className="w-4 h-4" />
                              </button>
                              <button
                                onClick={() => askDeleteUser(u)}
                                disabled={
                                  deleteUserMutation.isPending || !canDeleteUser(currentUser, u)
                                }
                                className="p-1.5 rounded-control text-muted hover:text-danger hover:bg-danger-wash border border-transparent hover:border-danger-edge transition inline-flex items-center align-middle disabled:opacity-30 disabled:hover:border-transparent disabled:hover:bg-transparent disabled:hover:text-muted"
                                title={
                                  canDeleteUser(currentUser, u)
                                    ? 'Delete User'
                                    : u.id === currentUser?.id
                                      ? 'You cannot delete your own account'
                                      : `A ${u.role} account is out of your reach`
                                }
                              >
                                <Trash2 className="w-4 h-4" />
                              </button>
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                  <InfiniteScrollSentinel
                    rootRef={usersScrollRef}
                    hasNextPage={hasMoreUsers}
                    isFetchingNextPage={isFetchingMoreUsers}
                    fetchNextPage={fetchMoreUsers}
                  />
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </NavigationShell>
  );
}
