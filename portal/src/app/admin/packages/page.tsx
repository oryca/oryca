'use client';

import React, { useState, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import NavigationShell from '@/components/NavigationShell';
import {
  Network,
  Users,
  Plus,
  Trash2,
  Edit,
  Save,
  CheckCircle,
  AlertTriangle,
  ChevronDown,
  Layers,
  Shield,
  Clock,
  UserCheck,
  Search,
  Server
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

interface PackageSvcLink {
  id: string;
  packageId: string;
  serviceId: string;
  paths?: PackagePath[];
}

interface User {
  id: string;
  email: string;
  firstName?: string;
  lastName?: string;
  role: 'user' | 'admin' | 'root';
  packageId?: string;
  verified?: boolean;
  enabled?: boolean;
}

export default function AdminPackagesPage() {
  const queryClient = useQueryClient();
  const [activeTab, setActiveTab] = useState<'packages' | 'users'>('packages');

  // Opening an account for someone, rather than waiting for them to sign up
  const [userSearch, setUserSearch] = useState('');
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
  const [linkMethods, setLinkMethods] = useState<string[]>(['GET']);
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
  const [success, setSuccess] = useState<string | null>(null);

  // Queries
  const { data: packages, isLoading: isLoadingPackages } = useQuery<Package[]>({
    queryKey: ['admin-packages'],
    queryFn: async () => {
      const res = await api.get('/packages');
      return res.data.items || [];
    },
  });

  const { data: services } = useQuery<any[]>({
    queryKey: ['admin-services-list'],
    queryFn: async () => {
      const res = await api.get('/services');
      return res.data.items || [];
    },
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

  const { data: users, isLoading: isLoadingUsers } = useQuery<User[]>({
    queryKey: ['admin-users', userSearch],
    queryFn: async () => {
      const res = await api.get('/users', {
        params: userSearch ? { search: userSearch } : undefined,
      });
      return res.data.items || [];
    },
    enabled: activeTab === 'users',
  });

  // Query Package Service links
  const { data: activeLinks, refetch: refetchLinks } = useQuery<PackageSvcLink[]>({
    queryKey: ['package-links', linkingPackage?.id],
    queryFn: async () => {
      if (!linkingPackage) return [];
      const res = await api.get(`/packages/${linkingPackage.id}/services`);
      return res.data.items || [];
    },
    enabled: !!linkingPackage && isLinkModalOpen,
  });

  // Load Package values into form
  useEffect(() => {
    if (editingPackage) {
      setPackageName(editingPackage.name || '');
      setPackageAlias(editingPackage.alias || '');
      setPackageDescription(editingPackage.description || '');
      setRateLimitEnabled(editingPackage.policies?.rateLimit?.enabled !== false);
      setTiers(editingPackage.policies?.rateLimit?.tiers || [{ limit: 10, windowSec: 60 }]);
      setIsPackageFormOpen(true);
    } else {
      setPackageName('');
      setPackageAlias('');
      setPackageDescription('');
      setRateLimitEnabled(true);
      setTiers([{ limit: 10, windowSec: 60 }]);
    }
    setFormError(null);
  }, [editingPackage, isPackageFormOpen]);

  // Load User values into form
  useEffect(() => {
    if (editingUser) {
      setUserRole(editingUser.role || 'user');
      setUserPackageId(editingUser.packageId || '');
      setUserVerified(editingUser.verified === true);
      setUserEnabled(editingUser.enabled !== false);
      setIsUserModalOpen(true);
    }
  }, [editingUser]);

  // Mutations
  const savePackageMutation = useMutation({
    mutationFn: async () => {
      setFormError(null);
      const payload = {
        name: packageName,
        alias: packageAlias,
        description: packageDescription,
        enabled: true,
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
      setSuccess('Package tier configuration saved.');
    },
    onError: (err: any) => {
      setFormError(err.message || 'Failed to save package.');
    },
  });

  const deletePackageMutation = useMutation({
    mutationFn: async (id: string) => {
      await api.delete(`/packages/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-packages'] });
      setSuccess('Package deleted.');
    },
  });

  // Package Service Links Mutations
  const linkServiceMutation = useMutation({
    mutationFn: async () => {
      if (!linkingPackage || !selectedServiceId) return;
      
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
    onError: (err: any) => {
      alert(err.message || 'Failed to grant service access.');
    },
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
      const payload = {
        firstName: editingUser.firstName || '',
        lastName: editingUser.lastName || '',
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
      setSuccess('User details updated successfully.');
    },
    onError: (err: any) => {
      setFormError(err.message || 'Failed to update user.');
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
    onError: (err: any) => {
      setNewUserError(err.message || 'Could not create the account.');
    },
  });

  const deleteUserMutation = useMutation({
    mutationFn: async (id: string) => {
      await api.delete(`/users/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-users'] });
      setSuccess('User account deleted.');
    },
  });

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

  return (
    <NavigationShell>
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
            📦 Package Tiers
          </button>
          <button
            onClick={() => setActiveTab('users')}
            className={`pb-3 text-sm font-semibold border-b-2 transition ${
              activeTab === 'users'
                ? 'border-accent text-ink font-bold'
                : 'border-transparent text-muted hover:text-ink'
            }`}
          >
            👥 User Management
          </button>
        </div>

        {success && (
          <div className="flex items-start gap-3 rounded-control border border-ok-edge bg-ok-wash p-4 text-xs text-ok">
            <CheckCircle className="w-4 h-4 mt-0.5 flex-shrink-0" />
            <div>{success}</div>
          </div>
        )}

        {/* ================================= PACKAGES TAB ================================= */}
        {activeTab === 'packages' && (
          <div className="space-y-6">
            <div className="flex justify-between items-center">
              <h2 className="text-xs font-bold text-muted uppercase tracking-wider">Access Limits & Tiers</h2>
              <button
                onClick={() => {
                  setEditingPackage(null);
                  setIsPackageFormOpen(true);
                }}
                className="px-3 py-1.5 bg-accent hover:bg-accent-deep text-accent-ink text-xs font-semibold rounded-control transition duration-short flex items-center gap-1"
              >
                <Plus className="w-4 h-4" /> Create Package
              </button>
            </div>

            {/* Package Form Modal */}
            {isPackageFormOpen && (
              <div className="fixed inset-0 z-50 bg-scrim flex items-center justify-center p-4">
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
                          Alias Key
                        </label>
                        <input
                          type="text"
                          required
                          value={packageAlias}
                          onChange={(e) => setPackageAlias(e.target.value)}
                          placeholder="e.g. free"
                          disabled={!!editingPackage}
                          className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus font-mono"
                        />
                      </div>
                      <div>
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          Display Name
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
                        onClick={() => {
                          setIsPackageFormOpen(false);
                          setEditingPackage(null);
                        }}
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
              <div className="fixed inset-0 z-50 bg-scrim flex items-center justify-center p-4">
                <div className="bg-paper border border-rule rounded-surface p-6 w-full max-w-xl shadow-sm space-y-4 max-h-[90vh] overflow-y-auto">
                  <div className="flex justify-between items-center border-b border-rule pb-2">
                    <h3 className="font-title text-base font-bold text-ink">
                      Link Services: {linkingPackage.name}
                    </h3>
                    <button onClick={() => setIsLinkModalOpen(false)} className="text-muted hover:text-ink">Close</button>
                  </div>

                  {/* Add Service Link form */}
                  <form
                    onSubmit={(e) => {
                      e.preventDefault();
                      linkServiceMutation.mutate();
                    }}
                    className="space-y-4 text-xs bg-paper-2 p-4 border border-rule rounded-surface"
                  >
                    <div className="grid grid-cols-2 gap-4">
                      <div>
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          Select Service
                        </label>
                        <select
                          required
                          value={selectedServiceId}
                          onChange={(e) => setSelectedServiceId(e.target.value)}
                          className="w-full bg-paper border border-rule rounded-control px-3 py-1.5 text-xs text-ink outline-none"
                        >
                          <option value="">Choose service...</option>
                          {services?.map((svc) => (
                            <option key={svc.id} value={svc.id}>
                              {svc.name} ({svc.basePath})
                            </option>
                          ))}
                        </select>
                      </div>

                      <div>
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          Path Rule
                        </label>
                        <input
                          type="text"
                          value={linkPath}
                          onChange={(e) => setLinkPath(e.target.value)}
                          className="w-full bg-paper border border-rule rounded-control px-3 py-1.5 text-xs text-ink outline-none font-mono"
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
                              <label className="text-[9px] font-semibold text-muted block mb-0.5">Requests</label>
                              <input
                                type="number"
                                required
                                value={tier.limit}
                                onChange={(e) => updateTier(index, 'limit', parseInt(e.target.value) || 1, true)}
                                className="w-full bg-paper border border-rule rounded-control px-2 py-0.5 text-xs font-mono"
                              />
                            </div>
                            <div className="flex-1">
                              <label className="text-[9px] font-semibold text-muted block mb-0.5">Window (Sec)</label>
                              <input
                                type="number"
                                required
                                value={tier.windowSec}
                                onChange={(e) => updateTier(index, 'windowSec', parseInt(e.target.value) || 1, true)}
                                className="w-full bg-paper border border-rule rounded-control px-2 py-0.5 text-xs font-mono"
                              />
                            </div>
                            <button
                              type="button"
                              onClick={() => removeTier(index, true)}
                              disabled={linkTiers.length <= 1}
                              className="p-1 text-muted hover:text-danger mt-4"
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
                      <p className="text-xs text-muted text-center py-4">This package has no access to any services yet.</p>
                    ) : (
                      <div className="divide-y divide-rule border border-rule rounded-surface overflow-hidden bg-paper">
                        {activeLinks?.map((link) => {
                          const svc = services?.find((s) => s.id === link.serviceId);
                          return (
                            <div key={link.id} className="p-3 flex items-center justify-between gap-4 text-xs hover:bg-paper-2 transition">
                              <div className="flex items-center gap-2">
                                <Server className="w-4 h-4 text-accent" />
                                <div>
                                  <p className="font-semibold text-ink">{svc?.name || 'API Service'}</p>
                                  <p className="text-[10px] text-muted font-mono">{link.paths?.[0]?.path || '/*'}</p>
                                </div>
                              </div>
                              <button
                                onClick={() => unlinkServiceMutation.mutate(link.serviceId)}
                                className="text-xs font-semibold text-danger hover:underline"
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
              <div className="space-y-4">
                {[...Array(2)].map((_, i) => (
                  <div key={i} className="h-16 bg-paper rounded-surface border border-rule animate-pulse"></div>
                ))}
              </div>
            ) : !packages || packages.length === 0 ? (
              <div className="p-8 bg-paper border border-rule border-dashed rounded-surface text-center text-xs text-muted">
                No package tiers created. Create a package to define access rate limits.
              </div>
            ) : (
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
                          onClick={() => setEditingPackage(pkg)}
                          className="p-1.5 rounded-control text-muted hover:text-ink hover:bg-paper-3 transition"
                          title="Edit Package"
                        >
                          <Edit className="w-4 h-4" />
                        </button>
                        <button
                          onClick={() => {
                            if (confirm(`Delete package tier "${pkg.name}"?`)) {
                              deletePackageMutation.mutate(pkg.id);
                            }
                          }}
                          disabled={deletePackageMutation.isPending}
                          className="p-1.5 rounded-control text-muted hover:text-danger hover:bg-danger-wash border border-transparent hover:border-danger-edge transition"
                          title="Delete Package"
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      </div>
                    </div>
                  </div>
                ))}
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
                <input
                  value={userSearch}
                  onChange={(e) => setUserSearch(e.target.value)}
                  placeholder="Search by email or name"
                  className="bg-paper-2 border border-rule rounded-control px-3 py-1.5 text-xs text-ink outline-none focus:border-focus w-56"
                />
                <button
                  type="button"
                  onClick={() => {
                    setNewUserError(null);
                    const fallback = packages?.find((p) => p.alias === defaultPackageAlias);
                    setNewPackageId(fallback?.id || '');
                    setIsNewUserOpen(true);
                  }}
                  className="px-3 py-1.5 rounded-control bg-accent text-white text-xs font-semibold"
                >
                  New user
                </button>
              </div>
            </div>

            {isNewUserOpen && (
              <div className="fixed inset-0 z-50 bg-scrim flex items-center justify-center p-4">
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
                    <input
                      required
                      type="email"
                      value={newEmail}
                      onChange={(e) => setNewEmail(e.target.value)}
                      placeholder="Email address"
                      className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                    />
                    <div className="flex gap-2">
                      <input
                        required
                        value={newFirstName}
                        onChange={(e) => setNewFirstName(e.target.value)}
                        placeholder="First name"
                        className="flex-1 min-w-0 bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                      />
                      <input
                        required
                        value={newLastName}
                        onChange={(e) => setNewLastName(e.target.value)}
                        placeholder="Last name"
                        className="flex-1 min-w-0 bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                      />
                    </div>
                    <input
                      required
                      type="text"
                      value={newPassword}
                      onChange={(e) => setNewPassword(e.target.value)}
                      placeholder="Password to hand over"
                      className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus font-mono"
                    />
                    <div className="flex gap-2">
                      <select
                        value={newRole}
                        onChange={(e) => setNewRole(e.target.value as 'user' | 'admin')}
                        className="flex-1 bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                      >
                        <option value="user">User</option>
                        <option value="admin">Administrator</option>
                      </select>
                      <select
                        value={newPackageId}
                        onChange={(e) => setNewPackageId(e.target.value)}
                        className="flex-1 bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus font-mono"
                      >
                        <option value="">No package (cannot call anything)</option>
                        {packages?.map((pkg) => (
                          <option key={pkg.id} value={pkg.id}>
                            {pkg.name} ({pkg.alias})
                          </option>
                        ))}
                      </select>
                    </div>

                    <div className="flex justify-end gap-3 pt-1">
                      <button
                        type="button"
                        onClick={() => setIsNewUserOpen(false)}
                        className="px-4 py-2 text-xs font-semibold text-muted"
                      >
                        Cancel
                      </button>
                      <button
                        type="submit"
                        disabled={createUserMutation.isPending}
                        className="px-4 py-2 rounded-control bg-accent text-white text-xs font-semibold disabled:opacity-50"
                      >
                        {createUserMutation.isPending ? 'Creating...' : 'Create account'}
                      </button>
                    </div>
                  </form>
                </div>
              </div>
            )}

            {/* Edit User Modal */}
            {isUserModalOpen && editingUser && (
              <div className="fixed inset-0 z-50 bg-scrim flex items-center justify-center p-4">
                <div className="bg-paper border border-rule rounded-surface p-6 w-full max-w-md shadow-sm space-y-4 max-h-[90vh] overflow-y-auto">
                  <h3 className="font-title text-base font-bold text-ink">
                    Edit User Profile: {editingUser.email}
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
                      saveUserMutation.mutate();
                    }}
                    className="space-y-4 text-xs"
                  >
                    <div>
                      <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                        System Role
                      </label>
                      <select
                        value={userRole}
                        onChange={(e) => setUserRole(e.target.value as any)}
                        disabled={editingUser.role === 'root'} // Disable root editing for safety
                        className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                      >
                        <option value="user">User</option>
                        <option value="admin">Administrator</option>
                        {editingUser.role === 'root' && <option value="root">Root</option>}
                      </select>
                    </div>

                    <div>
                      <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                        Assigned Package Tier
                      </label>
                      <select
                        value={userPackageId}
                        onChange={(e) => setUserPackageId(e.target.value)}
                        className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus font-mono"
                      >
                        <option value="">No Package Assigned</option>
                        {packages?.map((pkg) => (
                          <option key={pkg.id} value={pkg.id}>
                            {pkg.name} ({pkg.alias})
                          </option>
                        ))}
                      </select>
                    </div>

                    <div className="flex flex-col gap-2 pt-2 border-t border-rule">
                      <div className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          id="usr-verified"
                          checked={userVerified}
                          onChange={(e) => setUserVerified(e.target.checked)}
                          disabled={editingUser.role === 'root'}
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
                          disabled={editingUser.role === 'root'}
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
              <div className="space-y-4">
                {[...Array(3)].map((_, i) => (
                  <div key={i} className="h-10 bg-paper-2 rounded-control animate-pulse"></div>
                ))}
              </div>
            ) : !users || users.length === 0 ? (
              <p className="text-xs text-muted text-center py-6">
                {userSearch ? `Nobody matches "${userSearch}".` : 'No users yet.'}
              </p>
            ) : (
              <div className="bg-paper border border-rule rounded-surface overflow-hidden shadow-sm">
                <div className="overflow-x-auto">
                  <table className="w-full text-left border-collapse text-xs">
                    <thead>
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
                        const pkg = packages?.find((p) => p.id === u.packageId);
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
                                onClick={() => setEditingUser(u)}
                                className="p-1.5 rounded-control text-muted hover:text-ink hover:bg-paper-3 transition inline-flex items-center align-middle"
                                title="Edit User"
                              >
                                <Edit className="w-4 h-4" />
                              </button>
                              <button
                                onClick={() => {
                                  if (confirm(`Delete user account "${u.email}" permanently?`)) {
                                    deleteUserMutation.mutate(u.id);
                                  }
                                }}
                                disabled={deleteUserMutation.isPending || u.role === 'root'}
                                className="p-1.5 rounded-control text-muted hover:text-danger hover:bg-danger-wash border border-transparent hover:border-danger-edge transition inline-flex items-center align-middle disabled:opacity-30"
                                title="Delete User"
                              >
                                <Trash2 className="w-4 h-4" />
                              </button>
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </NavigationShell>
  );
}
