'use client';

import React, { createContext, useContext, useEffect, useState, useRef } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  api,
  UserSession,
  getUserSession,
  setAuthData,
  clearAuthData,
  getAccessToken,
} from '@/lib/api';

export interface AppConfig {
  id?: string;
  terms?: string;
  privacyPolicy?: string;
  register?: {
    enabled: boolean;
    trialExpiresIn?: number;
    defaultPackageAlias?: string;
  };
}

interface AuthContextType {
  user: UserSession | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  config: AppConfig | null;
  isLoadingConfig: boolean;
  login: (authResponse: any) => void;
  logout: () => void;
  refreshUser: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}

export function Providers({ children }: { children: React.ReactNode }) {
  const [queryClient] = useState(() => new QueryClient({
    defaultOptions: {
      queries: {
        refetchOnWindowFocus: false,
        retry: false,
      },
    },
  }));

  const [user, setUser] = useState<UserSession | null>(null);
  const [isAuthenticated, setIsAuthenticated] = useState<boolean>(false);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [isLoadingConfig, setIsLoadingConfig] = useState<boolean>(true);
  const initialized = useRef(false);

  useEffect(() => {
    if (typeof window !== 'undefined') {
      const savedUser = getUserSession();
      const token = getAccessToken();
      if (savedUser && token) {
        setUser(savedUser);
        setIsAuthenticated(true);
      }
      setIsLoading(false);
    }
  }, []);

  // Fetch app configuration (GET /configuration)
  useEffect(() => {
    if (initialized.current) return;
    initialized.current = true;

    async function fetchConfig() {
      try {
        const res = await api.get<AppConfig>('/configuration');
        setConfig(res.data);
      } catch (err) {
        console.error('Failed to fetch configurations:', err);
      } finally {
        setIsLoadingConfig(false);
      }
    }
    fetchConfig();
  }, []);

  const login = (authResponse: any) => {
    setAuthData(authResponse);
    setUser(authResponse.user);
    setIsAuthenticated(true);
  };

  const logout = async () => {
    try {
      await api.post('/auth/logout');
    } catch {
      // Ignored
    } finally {
      clearAuthData();
      setUser(null);
      setIsAuthenticated(false);
      window.location.href = '/auth/login';
    }
  };

  const refreshUser = async () => {
    try {
      const res = await api.get('/account/profile');
      const updatedUser = {
        id: res.data.id,
        email: res.data.email,
        role: res.data.role,
        firstName: res.data.firstName,
        lastName: res.data.lastName,
        displayName: res.data.displayName,
        packageId: res.data.packageId,
      };
      setUser(updatedUser);
      localStorage.setItem('oryca_user', JSON.stringify(updatedUser));
    } catch (err) {
      console.error('Failed to refresh user profile:', err);
    }
  };

  return (
    <QueryClientProvider client={queryClient}>
      <AuthContext.Provider
        value={{
          user,
          isAuthenticated,
          isLoading,
          config,
          isLoadingConfig,
          login,
          logout,
          refreshUser,
        }}
      >
        {children}
      </AuthContext.Provider>
    </QueryClientProvider>
  );
}
