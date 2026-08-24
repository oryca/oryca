/* Hallmark · macrostructure: Letter · genre: modern-minimal
 * design-system: design.md · designed-as-app
 */
'use client';

import React, { useEffect, useState, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import axios from 'axios';
import { useAuth } from '@/app/providers';
import { api } from '@/lib/api';
import { AuthShell, AuthLink, Button, Field, Notice, SkeletonLine } from '@/components/ui';
import { Shield } from 'lucide-react';

/** ข้อความที่พามาจากหน้าอื่นผ่าน query string */
const HANDOFF: Record<string, { tone: 'ok' | 'info'; text: string }> = {
  expired: { tone: 'info', text: 'Your session expired. Sign in again.' },
  registered: { tone: 'ok', text: 'Account created. You can sign in now.' },
  registered_pending: {
    tone: 'info',
    text: 'Account created. An administrator has to verify it before you can sign in.',
  },
};

function LoginForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { login, isAuthenticated } = useAuth();

  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [isNotVerified, setIsNotVerified] = useState(false);
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    if (isAuthenticated) router.push('/dashboard');
  }, [isAuthenticated, router]);

  const handoffKey = Object.keys(HANDOFF).find((k) => searchParams.get(k));
  const handoff = handoffKey ? HANDOFF[handoffKey] : null;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setIsNotVerified(false);
    setIsLoading(true);

    try {
      const res = await api.post('/auth/login', { username, password });
      login(res.data);
      router.push('/dashboard');
    } catch (err) {
      const notVerified =
        axios.isAxiosError(err) &&
        err.response?.status === 403 &&
        err.response?.data?.code === 'USER_NOT_VERIFIED';

      if (notVerified) {
        setIsNotVerified(true);
        setError('This account is not verified yet. Ask an administrator to verify it.');
      } else {
        setError(
          err instanceof Error && err.message
            ? err.message
            : 'That username or password is not right.',
        );
      }
    } finally {
      setIsLoading(false);
    }
  }

  return (
    <AuthShell
      icon={<Shield className="h-6 w-6" />}
      title="Sign in to ORYCA"
      description="Developer portal and control panel for the API gateway"
      footer={
        <>
          No account yet? <AuthLink href="/auth/register">Create one</AuthLink>
        </>
      }
    >
      {handoff && <Notice tone={handoff.tone}>{handoff.text}</Notice>}
      {error && <Notice tone={isNotVerified ? 'warn' : 'danger'}>{error}</Notice>}

      <form className="space-y-4" onSubmit={handleSubmit}>
        <Field
          label="Email or username"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          placeholder="email@example.com"
          autoComplete="username"
          required
        />

        <div>
          <Field
            label="Password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            required
          />
          <p className="-mt-1 text-right text-xs text-muted">
            Forgot your password? Ask an administrator to set a new one.
          </p>
        </div>

        <Button type="submit" variant="primary" block loading={isLoading}>
          Sign in
        </Button>
      </form>
    </AuthShell>
  );
}

export default function LoginPage() {
  return (
    <Suspense
      fallback={
        <AuthShell
          icon={<Shield className="h-6 w-6" />}
          title="Sign in to ORYCA"
          description="Developer portal and control panel for the API gateway"
        >
          <SkeletonLine className="h-10" />
          <SkeletonLine className="h-10" />
          <SkeletonLine className="h-10" />
        </AuthShell>
      }
    >
      <LoginForm />
    </Suspense>
  );
}
