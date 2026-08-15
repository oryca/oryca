/* Hallmark · macrostructure: Letter · genre: modern-minimal
 * design-system: design.md · designed-as-app
 */
'use client';

import React, { useEffect, useState, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { api } from '@/lib/api';
import { AuthShell, AuthLink, Button, Field, Notice, SkeletonLine } from '@/components/ui';
import { KeyRound } from 'lucide-react';

interface TokenUser {
  email: string;
  firstName: string;
}

function SetPasswordForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const token = searchParams.get('token');

  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [confirmTouched, setConfirmTouched] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  // ไม่มีโทเคนคือสิ่งที่รู้ได้ตั้งแต่ render แรก ไม่ต้องผ่าน effect
  const [isValidating, setIsValidating] = useState(Boolean(token));
  const [tokenUser, setTokenUser] = useState<TokenUser | null>(null);
  const [isTokenValid, setIsTokenValid] = useState(false);

  useEffect(() => {
    if (!token) return;

    let cancelled = false;
    async function validate() {
      try {
        const res = await api.get(`/auth/set-password?token=${token}`);
        if (cancelled) return;
        setTokenUser(res.data);
        setIsTokenValid(true);
      } catch (err) {
        if (cancelled) return;
        setError(
          err instanceof Error && err.message
            ? err.message
            : 'That token is not valid, or it has expired.',
        );
      } finally {
        if (!cancelled) setIsValidating(false);
      }
    }
    validate();

    return () => {
      cancelled = true;
    };
  }, [token]);

  const mismatch = confirmTouched && confirmPassword.length > 0 && password !== confirmPassword;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setConfirmTouched(true);
    if (password !== confirmPassword) return;

    setError(null);
    setIsLoading(true);
    try {
      await api.post('/auth/set-password', { token, password });
      router.push('/auth/login?reset_success=true');
    } catch (err) {
      setError(
        err instanceof Error && err.message ? err.message : 'Could not set the password. Try again.',
      );
    } finally {
      setIsLoading(false);
    }
  }

  if (isValidating) {
    return (
      <>
        <SkeletonLine className="h-10" />
        <SkeletonLine className="h-10" />
      </>
    );
  }

  if (!isTokenValid) {
    return (
      <Notice tone="danger" title="This link will not work">
        {error ?? 'The link has no token, or the token was lost on the way.'}
        <br />
        Request a fresh one from the forgot-password page.
      </Notice>
    );
  }

  return (
    <form className="space-y-4" onSubmit={handleSubmit}>
      {tokenUser && (
        <Notice tone="info">
          Setting a new password for{' '}
          <span className="font-medium text-ink">
            {tokenUser.firstName} ({tokenUser.email})
          </span>
        </Notice>
      )}

      {error && <Notice tone="danger">{error}</Notice>}

      <Field
        label="New password"
        type="password"
        value={password}
        onChange={(e) => setPassword(e.target.value)}
        autoComplete="new-password"
        required
      />

      <Field
        label="Confirm new password"
        type="password"
        value={confirmPassword}
        onChange={(e) => setConfirmPassword(e.target.value)}
        onBlur={() => setConfirmTouched(true)}
        error={mismatch ? 'The two passwords do not match. Type them again.' : undefined}
        autoComplete="new-password"
        required
      />

      <Button type="submit" variant="primary" block loading={isLoading} disabled={mismatch}>
        Set password
      </Button>
    </form>
  );
}

export default function SetPasswordPage() {
  return (
    <AuthShell
      icon={<KeyRound className="h-6 w-6" />}
      title="Set a new password"
      description="Choose the password you will sign in with next time"
      footer={<AuthLink href="/auth/login">Back to sign in</AuthLink>}
    >
      <Suspense
        fallback={
          <>
            <SkeletonLine className="h-10" />
            <SkeletonLine className="h-10" />
          </>
        }
      >
        <SetPasswordForm />
      </Suspense>
    </AuthShell>
  );
}
