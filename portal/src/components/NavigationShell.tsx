'use client';

import React, { useState, useEffect } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import Link from 'next/link';
import { useAuth } from '@/app/providers';
import { api } from '@/lib/api';
import {
  LayoutDashboard,
  BookOpen,
  User,
  Bell,
  Network,
  GitBranch,
  Layers,
  Mail,
  LogOut,
  Menu,
  X,
  Database,
  Key
} from 'lucide-react';

interface NavItem {
  name: string;
  href: string;
  icon: React.ComponentType<any>;
  roles: ('user' | 'admin' | 'root')[];
}

const navItems: NavItem[] = [
  { name: 'Dashboard', href: '/dashboard', icon: LayoutDashboard, roles: ['user', 'admin', 'root'] },
  { name: 'Services & Keys', href: '/services', icon: Key, roles: ['user', 'admin', 'root'] },
  { name: 'Profile & Sessions', href: '/profile', icon: User, roles: ['user', 'admin', 'root'] },
  { name: 'Notifications', href: '/notifications', icon: Bell, roles: ['user', 'admin', 'root'] },
  
  // Admin & Root only
  { name: 'Manage Services', href: '/admin/services', icon: GitBranch, roles: ['admin', 'root'] },
  { name: 'Transforms & Presets', href: '/admin/transforms', icon: Layers, roles: ['admin', 'root'] },
  { name: 'Packages & Users', href: '/admin/packages', icon: Network, roles: ['admin', 'root'] },
  { name: 'Mail settings', href: '/admin/mail', icon: Mail, roles: ['admin', 'root'] },
];

export default function NavigationShell({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const { user, isAuthenticated, isLoading, logout } = useAuth();
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);
  const [unreadCount, setUnreadCount] = useState(0);

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      router.push('/auth/login');
    }
  }, [isLoading, isAuthenticated, router]);

  // Fetch unread notification count
  useEffect(() => {
    if (isAuthenticated) {
      async function getUnread() {
        try {
          const res = await api.get('/notifications/unread-count');
          setUnreadCount(res.data.unreadCount || 0);
        } catch {
          // Ignored
        }
      }
      getUnread();
      
      // Set interval to fetch count every 15s
      const interval = setInterval(getUnread, 15000);
      return () => clearInterval(interval);
    }
  }, [isAuthenticated]);

  if (isLoading || !user) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-paper-2">
        <div className="w-8 h-8 border-2 border-rule border-t-accent rounded-full animate-spin"></div>
      </div>
    );
  }

  const visibleItems = navItems.filter((item) => item.roles.includes(user.role));

  return (
    <div className="flex min-h-screen bg-paper-2">
      {/* Sidebar for Desktop */}
      <aside className="hidden md:flex md:w-64 md:flex-col md:fixed md:inset-y-0 bg-paper border-r border-rule">
        <div className="flex flex-col flex-grow pt-5 pb-4 overflow-y-auto">
          <div className="flex items-center flex-shrink-0 px-6 gap-2 border-b border-rule pb-5">
            <span className="font-title text-xl font-bold tracking-tight text-ink">
              oryca<span className="text-accent">.</span>portal
            </span>
            <span className="text-[10px] uppercase font-bold tracking-wider px-2 py-0.5 rounded-chip bg-accent-wash border border-accent-edge text-accent">
              {user.role}
            </span>
          </div>
          
          <nav className="mt-6 flex-1 px-4 space-y-1">
            {visibleItems.map((item) => {
              const isActive = pathname.startsWith(item.href);
              const Icon = item.icon;
              return (
                <Link
                  key={item.name}
                  href={item.href}
                  className={`group flex items-center px-3 py-2.5 text-sm font-medium rounded-control transition duration-short ${
                    isActive
                      ? 'bg-accent-wash border border-accent-edge text-accent'
                      : 'text-ink-2 hover:bg-paper-3 border border-transparent'
                  }`}
                >
                  <Icon
                    className={`mr-3 flex-shrink-0 h-5 w-5 ${
                      isActive ? 'text-accent' : 'text-muted group-hover:text-ink-2'
                    }`}
                  />
                  <span className="flex-1">{item.name}</span>
                  {item.name === 'Notifications' && unreadCount > 0 && (
                    <span className="ml-2 bg-accent text-accent-ink text-[10px] font-bold px-2 py-0.5 rounded-full">
                      {unreadCount}
                    </span>
                  )}
                </Link>
              );
            })}
          </nav>
        </div>

        {/* Sidebar Footer */}
        <div className="flex-shrink-0 flex border-t border-rule p-4 bg-paper-2">
          <div className="flex items-center justify-between w-full">
            <div className="flex items-center min-w-0">
              <div className="w-9 h-9 rounded-full bg-accent-wash border border-accent-edge flex items-center justify-center text-accent font-title font-semibold uppercase">
                {user.email.substring(0, 2)}
              </div>
              <div className="ml-3 min-w-0">
                <p className="text-sm font-semibold text-ink truncate">
                  {user.displayName || user.email.split('@')[0]}
                </p>
                <p className="text-xs text-muted truncate">{user.email}</p>
              </div>
            </div>
            <button
              onClick={logout}
              className="p-1.5 rounded-control text-muted hover:text-danger hover:bg-danger-wash border border-transparent hover:border-danger-edge transition duration-short"
              title="Logout"
            >
              <LogOut className="h-5 w-5" />
            </button>
          </div>
        </div>
      </aside>

      {/* Main Container */}
      <div className="md:pl-64 flex flex-col flex-1 w-full">
        {/* Mobile Header */}
        <header className="sticky top-0 z-40 flex items-center justify-between h-14 bg-paper border-b border-rule px-4 md:hidden">
          <span className="font-title text-lg font-bold tracking-tight text-ink">
            oryca<span className="text-accent">.</span>portal
          </span>
          <button
            onClick={() => setIsMobileMenuOpen(!isMobileMenuOpen)}
            className="p-1 rounded-control text-ink-2 border border-rule"
          >
            {isMobileMenuOpen ? <X className="h-6 w-6" /> : <Menu className="h-6 w-6" />}
          </button>
        </header>

        {/* Mobile Drawer Menu */}
        {isMobileMenuOpen && (
          <div className="fixed inset-0 z-30 md:hidden bg-scrim" onClick={() => setIsMobileMenuOpen(false)}>
            <div className="fixed inset-y-0 right-0 w-64 bg-paper border-l border-rule flex flex-col p-4 space-y-4" onClick={(e) => e.stopPropagation()}>
              <div className="flex items-center justify-between pb-3 border-b border-rule">
                <span className="font-title text-lg font-bold tracking-tight text-ink">Navigation</span>
                <button onClick={() => setIsMobileMenuOpen(false)} className="p-1 text-muted">
                  <X className="h-5 w-5" />
                </button>
              </div>
              <nav className="flex-grow space-y-1">
                {visibleItems.map((item) => {
                  const isActive = pathname.startsWith(item.href);
                  const Icon = item.icon;
                  return (
                    <Link
                      key={item.name}
                      href={item.href}
                      onClick={() => setIsMobileMenuOpen(false)}
                      className={`flex items-center px-3 py-2 text-sm font-medium rounded-control ${
                        isActive
                          ? 'bg-accent-wash text-accent border border-accent-edge'
                          : 'text-ink-2 hover:bg-paper-2 border border-transparent'
                      }`}
                    >
                      <Icon className="mr-3 h-5 w-5 flex-shrink-0" />
                      <span className="flex-1">{item.name}</span>
                      {item.name === 'Notifications' && unreadCount > 0 && (
                        <span className="ml-2 bg-accent text-accent-ink text-[10px] font-bold px-2 py-0.5 rounded-full">
                          {unreadCount}
                        </span>
                      )}
                    </Link>
                  );
                })}
              </nav>
              <div className="border-t border-rule pt-4 flex items-center justify-between">
                <div className="flex items-center min-w-0">
                  <div className="w-8 h-8 rounded-full bg-accent-wash flex items-center justify-center text-accent font-semibold uppercase">
                    {user.email.substring(0, 2)}
                  </div>
                  <span className="ml-2 text-sm text-ink font-semibold truncate">{user.email}</span>
                </div>
                <button onClick={logout} className="p-1 text-danger hover:bg-danger-wash rounded-control">
                  <LogOut className="h-5 w-5" />
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Main Content Area */}
        <main className="flex-grow p-4 md:p-8 max-w-7xl w-full mx-auto">
          {children}
        </main>
      </div>
    </div>
  );
}
