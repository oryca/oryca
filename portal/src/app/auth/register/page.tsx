/* Hallmark · macrostructure: Letter · genre: modern-minimal
 * design-system: design.md · designed-as-app
 */
'use client';

import React, { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { useAuth } from '@/app/providers';
import { api } from '@/lib/api';
import { AuthShell, AuthLink, Button, Field, Notice, SkeletonLine } from '@/components/ui';
import { UserPlus } from 'lucide-react';

export default function RegisterPage() {
  const router = useRouter();
  const { config, isLoadingConfig, isAuthenticated } = useAuth();

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [firstName, setFirstName] = useState('');
  const [lastName, setLastName] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    if (isAuthenticated) router.push('/dashboard');
  }, [isAuthenticated, router]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setIsLoading(true);

    try {
      const res = await api.post('/auth/register', { email, password, firstName, lastName });
      // an unverified account waits for an administrator, so say so instead of "sign in now"
      const handoff = res.data?.verified ? 'registered' : 'registered_pending';
      router.push(`/auth/login?${handoff}=true`);
    } catch (err) {
      setError(
        err instanceof Error && err.message ? err.message : 'Could not create the account. Try again.',
      );
    } finally {
      setIsLoading(false);
    }
  }

  if (isLoadingConfig) {
    return (
      <AuthShell
        icon={<UserPlus className="h-6 w-6" />}
        title="Create an account"
        description="Request access to the geospatial APIs"
      >
        <SkeletonLine className="h-10" />
        <SkeletonLine className="h-10" />
        <SkeletonLine className="h-10" />
      </AuthShell>
    );
  }

  const registrationOpen = config?.register?.enabled !== false;

  return (
    <AuthShell
      icon={<UserPlus className="h-6 w-6" />}
      title="Create an account"
      description="Request access to the geospatial APIs"
      footer={
        registrationOpen ? (
          <>
            Already have an account? <AuthLink href="/auth/login">Sign in</AuthLink>
          </>
        ) : (
          <AuthLink href="/auth/login">Back to sign in</AuthLink>
        )
      }
    >
      {!registrationOpen && (
        <Notice tone="warn" title="Registration is closed">
          New accounts are turned off right now. Contact an administrator if you need access.
        </Notice>
      )}

      {registrationOpen && (
        <>
          {error && <Notice tone="danger">{error}</Notice>}

          <form className="space-y-4" onSubmit={handleSubmit}>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field
                label="First name"
                value={firstName}
                onChange={(e) => setFirstName(e.target.value)}
                placeholder="Somchai"
                autoComplete="given-name"
                required
              />
              <Field
                label="Last name"
                value={lastName}
                onChange={(e) => setLastName(e.target.value)}
                placeholder="Jaidee"
                autoComplete="family-name"
                required
              />
            </div>

            <Field
              label="Email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@example.com"
              autoComplete="email"
              required
            />

            <Field
              label="Password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="new-password"
              required
            />

            <Button type="submit" variant="primary" block loading={isLoading}>
              Create account
            </Button>
          </form>
        </>
      )}
    </AuthShell>
  );
}
