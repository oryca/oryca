/* Hallmark · genre: modern-minimal · macrostructure: Workbench
 * design-system: design.md · designed-as-app
 * เมนูจัดกลุ่มตามงาน ไม่ใช่ list แบน · drawer มือถือ: Esc · scroll lock · focus trap
 */
'use client';

import React, { useState, useEffect, useRef, useCallback } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import Link from 'next/link';
import { useAuth } from '@/app/providers';
import { api } from '@/lib/api';
import {
  LayoutDashboard,
  User,
  Bell,
  Network,
  GitBranch,
  Layers,
  LogOut,
  Menu,
  X,
  Key,
  TerminalSquare,
} from 'lucide-react';

type Role = 'user' | 'admin' | 'root';

interface NavItem {
  name: string;
  href: string;
  icon: React.ComponentType<{ className?: string }>;
  roles?: Role[];
}

interface NavGroup {
  label: string;
  roles: Role[];
  items: NavItem[];
}

const navGroups: NavGroup[] = [
  {
    label: 'Work',
    roles: ['user', 'admin', 'root'],
    items: [
      { name: 'Dashboard', href: '/dashboard', icon: LayoutDashboard },
      { name: 'Services & Keys', href: '/services', icon: Key, roles: ['user', 'admin'] },
      { name: 'Sandbox', href: '/sandbox', icon: TerminalSquare, roles: ['user', 'admin'] },
    ],
  },
  {
    label: 'Account',
    roles: ['user', 'admin', 'root'],
    items: [
      { name: 'Profile & Sessions', href: '/profile', icon: User },
      { name: 'Notifications', href: '/notifications', icon: Bell },
    ],
  },
  {
    label: 'Administration',
    roles: ['admin', 'root'],
    items: [
      { name: 'Manage Services', href: '/admin/services', icon: GitBranch },
      { name: 'Transforms & Presets', href: '/admin/transforms', icon: Layers },
      { name: 'Packages & Users', href: '/admin/packages', icon: Network },
    ],
  },
];

function allowedRoles(pathname: string): Role[] | null {
  for (const group of navGroups) {
    for (const item of group.items) {
      if (pathname === item.href || pathname.startsWith(`${item.href}/`)) {
        return item.roles ?? group.roles;
      }
    }
  }
  return null;
}

function NavLink({
  item,
  pathname,
  unreadCount,
  onNavigate,
}: {
  item: NavItem;
  pathname: string;
  unreadCount: number;
  onNavigate?: () => void;
}) {
  const isActive = pathname.startsWith(item.href);
  const Icon = item.icon;
  const showBadge = item.href === '/notifications' && unreadCount > 0;

  return (
    <Link
      href={item.href}
      onClick={onNavigate}
      aria-current={isActive ? 'page' : undefined}
      className={`flex items-center gap-3 rounded-control border px-3 py-2 text-sm transition-colors duration-200 ${
        isActive
          ? 'border-accent-edge bg-accent-wash font-medium text-accent'
          : 'border-transparent text-ink-2 hover:bg-paper-3 hover:text-ink'
      }`}
    >
      <Icon className={`h-[18px] w-[18px] shrink-0 ${isActive ? 'text-accent' : 'text-muted'}`} />
      <span className="min-w-0 flex-1 truncate">{item.name}</span>
      {showBadge && (
        <span className="tabular shrink-0 rounded-full bg-accent px-1.5 py-0.5 text-[11px] font-semibold text-accent-ink">
          {unreadCount}
        </span>
      )}
    </Link>
  );
}

function NavGroups({
  groups,
  pathname,
  unreadCount,
  onNavigate,
}: {
  groups: NavGroup[];
  pathname: string;
  unreadCount: number;
  onNavigate?: () => void;
}) {
  return (
    <>
      {groups.map((group) => (
        <div key={group.label} className="mb-5">
          <p className="mb-1.5 px-3 text-[11px] font-medium tracking-wide text-muted">
            {group.label}
          </p>
          <div className="space-y-0.5">
            {group.items.map((item) => (
              <NavLink
                key={item.href}
                item={item}
                pathname={pathname}
                unreadCount={unreadCount}
                onNavigate={onNavigate}
              />
            ))}
          </div>
        </div>
      ))}
    </>
  );
}

function AccountRow({
  email,
  name,
  compact = false,
  onLogout,
}: {
  email: string;
  name: string;
  compact?: boolean;
  onLogout: () => void;
}) {
  return (
    <div className="flex items-center justify-between gap-2">
      <div className="flex min-w-0 items-center gap-2.5">
        <span
          aria-hidden="true"
          className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full border border-accent-edge bg-accent-wash font-title text-sm font-semibold uppercase text-accent"
        >
          {email.substring(0, 2)}
        </span>
        <span className="min-w-0">
          <span className="block truncate text-sm font-medium text-ink">{name}</span>
          {!compact && <span className="block truncate text-xs text-muted">{email}</span>}
        </span>
      </div>
      <button
        type="button"
        onClick={onLogout}
        title="Sign out"
        aria-label="Sign out"
        className="ui-btn ui-btn--sm ui-btn--icon ui-btn--ghost"
      >
        <LogOut className="h-[18px] w-[18px]" />
      </button>
    </div>
  );
}

function Wordmark({ size = 'md' }: { size?: 'sm' | 'md' }) {
  return (
    <span className={`font-title font-semibold tracking-tight text-ink ${size === 'sm' ? 'text-base' : 'text-lg'}`}>
      oryca<span className="text-accent">.</span>portal
    </span>
  );
}

export default function NavigationShell({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const { user, isAuthenticated, isLoading, logout } = useAuth();
  const [isDrawerOpen, setIsDrawerOpen] = useState(false);
  const [unreadCount, setUnreadCount] = useState(0);

  const drawerRef = useRef<HTMLDialogElement>(null);
  const toggleRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      router.push('/auth/login');
    }
  }, [isLoading, isAuthenticated, router]);

  const deniedRoute = !!user && !(allowedRoles(pathname)?.includes(user.role) ?? true);
  useEffect(() => {
    if (deniedRoute) router.replace('/dashboard');
  }, [deniedRoute, router]);

  useEffect(() => {
    if (!isAuthenticated) return;

    let cancelled = false;
    async function getUnread() {
      try {
        const res = await api.get('/notifications/unread-count');
        if (!cancelled) setUnreadCount(res.data.count || 0);
      } catch {
        // เงียบไว้ — ตัวเลขแจ้งเตือนพลาดได้ ไม่ควรรบกวนผู้ใช้
      }
    }
    getUnread();
    const interval = setInterval(getUnread, 15000);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [isAuthenticated]);

  const closeDrawer = useCallback(() => {
    drawerRef.current?.close();
  }, []);

  // เปิดผ่าน showModal() — focus trap · Esc · inert พื้นหลัง มาจากเบราว์เซอร์
  // เหลือแค่ล็อก scroll ของ body ที่ยังต้องทำเอง
  useEffect(() => {
    const el = drawerRef.current;
    if (!el || !isDrawerOpen) return;

    el.showModal();
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';

    return () => {
      document.body.style.overflow = previousOverflow;
    };
  }, [isDrawerOpen]);

  if (isLoading || !user) {
    return (
      <output className="flex min-h-screen items-center justify-center bg-paper-2 text-sm text-muted">
        Checking your session…
      </output>
    );
  }

  if (deniedRoute) {
    return (
      <output className="flex min-h-screen items-center justify-center bg-paper-2 text-sm text-muted">
        That page is not yours to open. Taking you back to the dashboard…
      </output>
    );
  }

  const visibleGroups = navGroups
    .filter((g) => g.roles.includes(user.role))
    .map((g) => ({ ...g, items: g.items.filter((i) => !i.roles || i.roles.includes(user.role)) }))
    .filter((g) => g.items.length > 0);
  const accountName = user.displayName || user.email.split('@')[0];

  return (
    <div className="flex min-h-screen bg-paper-2">
      {/* ข้ามไปเนื้อหา — ลิงก์แรกของหน้าสำหรับผู้ใช้คีย์บอร์ด */}
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:left-3 focus:top-3 focus:z-50 focus:rounded-control focus:border focus:border-rule-2 focus:bg-paper focus:px-3 focus:py-2 focus:text-sm focus:text-ink"
      >
        Skip to main content
      </a>

      {/* Sidebar — desktop */}
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-[var(--rail-width)] flex-col border-r border-rule bg-paper md:flex">
        <div className="flex items-center gap-2 border-b border-rule px-5 py-4">
          <Wordmark />
          <span className="rounded-chip border border-accent-edge bg-accent-wash px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-accent">
            {user.role}
          </span>
        </div>

        <nav aria-label="Main menu" className="flex-1 overflow-y-auto px-3 py-4">
          <NavGroups groups={visibleGroups} pathname={pathname} unreadCount={unreadCount} />
        </nav>

        <div className="border-t border-rule bg-paper-2 p-3">
          <AccountRow email={user.email} name={accountName} onLogout={logout} />
        </div>
      </aside>

      <div className="relative flex min-h-screen w-full flex-1 flex-col overflow-hidden md:pl-[var(--rail-width)]">
        {/* Crisp Origami Facet Background for Portal Pages */}
        <svg
          aria-hidden="true"
          className="pointer-events-none absolute top-0 right-0 z-0 h-[520px] w-[520px] opacity-70 select-none"
          viewBox="0 0 500 500"
          fill="none"
          xmlns="http://www.w3.org/2000/svg"
        >
          <polygon points="500,0 320,60 420,220" fill="oklch(91% 0.045 254 / 0.4)" stroke="oklch(82% 0.07 254 / 0.5)" strokeWidth="0.75" />
          <polygon points="320,60 180,0 240,180" fill="oklch(93% 0.035 254 / 0.35)" stroke="oklch(85% 0.05 254 / 0.4)" strokeWidth="0.75" />
          <polygon points="320,60 240,180 420,220" fill="oklch(88% 0.06 254 / 0.45)" stroke="oklch(79% 0.09 254 / 0.6)" strokeWidth="0.75" />
          <polygon points="420,220 240,180 300,360" fill="oklch(92% 0.04 254 / 0.35)" stroke="oklch(82% 0.07 254 / 0.45)" strokeWidth="0.75" />
          <polygon points="500,0 420,220 500,300" fill="oklch(89% 0.055 254 / 0.4)" stroke="oklch(80% 0.08 254 / 0.5)" strokeWidth="0.75" />
          <polygon points="420,220 500,300 300,360" fill="oklch(94% 0.03 254 / 0.3)" stroke="oklch(84% 0.06 254 / 0.35)" strokeWidth="0.75" />
        </svg>

        <svg
          aria-hidden="true"
          className="pointer-events-none absolute right-0 bottom-0 z-0 h-[420px] w-[420px] opacity-60 select-none"
          viewBox="0 0 400 400"
          fill="none"
          xmlns="http://www.w3.org/2000/svg"
        >
          <polygon points="400,400 240,320 340,180" fill="oklch(93% 0.04 78 / 0.4)" stroke="oklch(83% 0.07 75 / 0.4)" strokeWidth="0.75" />
          <polygon points="240,320 120,400 200,220" fill="oklch(95% 0.03 78 / 0.35)" stroke="oklch(86% 0.05 75 / 0.35)" strokeWidth="0.75" />
          <polygon points="240,320 200,220 340,180" fill="oklch(91% 0.055 78 / 0.4)" stroke="oklch(81% 0.08 75 / 0.45)" strokeWidth="0.75" />
          <polygon points="340,180 200,220 280,80" fill="oklch(96% 0.02 78 / 0.3)" stroke="oklch(88% 0.04 75 / 0.3)" strokeWidth="0.75" />
        </svg>

        {/* Header — mobile */}
        <header className="sticky top-0 z-40 flex h-14 items-center justify-between border-b border-rule bg-paper px-4 md:hidden">
          <Wordmark size="sm" />
          <button
            ref={toggleRef}
            type="button"
            onClick={() => setIsDrawerOpen(true)}
            aria-expanded={isDrawerOpen}
            aria-label="Open menu"
            className="ui-btn ui-btn--sm ui-btn--icon ui-btn--secondary"
          >
            <Menu className="h-5 w-5" />
          </button>
        </header>

        {/* Drawer — mobile */}
        {isDrawerOpen && (
          <dialog
            ref={drawerRef}
            className="ui-dialog ui-drawer z-50"
            aria-label="Main menu"
            onClose={() => {
              setIsDrawerOpen(false);
              toggleRef.current?.focus();
            }}
            onClick={(e) => {
              if (e.target === drawerRef.current) closeDrawer();
            }}
          >
            <div className="flex items-center justify-between border-b border-rule px-4 py-3">
              <Wordmark size="sm" />
              <button
                type="button"
                onClick={closeDrawer}
                aria-label="Close menu"
                className="ui-btn ui-btn--sm ui-btn--icon ui-btn--ghost"
              >
                <X className="h-5 w-5" />
              </button>
            </div>

            <nav aria-label="Main menu" className="flex-1 overflow-y-auto px-3 py-4">
              <NavGroups
                groups={visibleGroups}
                pathname={pathname}
                unreadCount={unreadCount}
                onNavigate={closeDrawer}
              />
            </nav>

            <div className="border-t border-rule bg-paper-2 p-3">
              <AccountRow email={user.email} name={accountName} compact onLogout={logout} />
            </div>
          </dialog>
        )}

        <main id="main" className="relative mx-auto w-full max-w-7xl flex-grow p-4 md:p-8">
          {children}
        </main>
      </div>
    </div>
  );
}
