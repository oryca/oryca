import axios from 'axios';

// ที่อยู่ที่ browser ใช้เรียก backend — layout.tsx ฉีดเข้ามาตอนเสิร์ฟหน้า ไม่ใช่ตอน build
// image เดียวจึงใช้ได้ทุก domain แค่เปลี่ยน env แล้ว restart
declare global {
  interface Window {
    __ORYCA_CONFIG__?: { apiUrl?: string; gatewayUrl?: string };
  }
}

export const API_URL =
  (typeof window !== 'undefined' && window.__ORYCA_CONFIG__?.apiUrl) ||
  process.env.NEXT_PUBLIC_API_URL ||
  'http://localhost:9001/control-plane/api/v1';

export const GATEWAY_URL =
  (typeof window !== 'undefined' && window.__ORYCA_CONFIG__?.gatewayUrl) ||
  process.env.NEXT_PUBLIC_GATEWAY_URL ||
  'http://localhost:9002/gateway/api';

export interface UserSession {
  id: string;
  email: string;
  role: 'user' | 'admin' | 'root';
  firstName?: string;
  lastName?: string;
  displayName?: string;
  packageId?: string;
}

/** Rows per request for every list call. The backend's own default is 10. */
export const LIST_PAGE_SIZE = 10;

// Envelope every control-plane list endpoint returns.
// numberMatched is the total behind the filter, ignoring limit/offset.
export interface ListResponse<T> {
  numberMatched: number;
  numberReturned: number;
  items: T[];
}

export interface AuthResponse {
  user: UserSession;
  accessToken: string;
  refreshToken: string;
  tokenType: string;
  expiresIn: number;
  expiredAt: string;
}

export const api = axios.create({
  baseURL: API_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Helper functions for Auth tokens in localStorage
export const getAccessToken = () => typeof window !== 'undefined' ? localStorage.getItem('oryca_access_token') : null;
export const getRefreshToken = () => typeof window !== 'undefined' ? localStorage.getItem('oryca_refresh_token') : null;
export const getUserSession = (): UserSession | null => {
  if (typeof window === 'undefined') return null;
  const userStr = localStorage.getItem('oryca_user');
  try {
    return userStr ? JSON.parse(userStr) : null;
  } catch {
    return null;
  }
};

export const setAuthData = (data: AuthResponse) => {
  localStorage.setItem('oryca_access_token', data.accessToken);
  localStorage.setItem('oryca_refresh_token', data.refreshToken);
  localStorage.setItem('oryca_user', JSON.stringify(data.user));
};

export const clearAuthData = () => {
  localStorage.removeItem('oryca_access_token');
  localStorage.removeItem('oryca_refresh_token');
  localStorage.removeItem('oryca_user');
};

// Add auth headers interceptor
api.interceptors.request.use((config) => {
  const token = getAccessToken();
  if (token && config.headers) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
}, (error) => {
  return Promise.reject(error);
});

// Flag to prevent infinite retry loops during refresh
let isRefreshing = false;

/** A request parked while the token refreshes, replayed once it lands. */
interface PendingRequest {
  resolve: (token: string | null) => void;
  reject: (reason: unknown) => void;
}
let failedQueue: PendingRequest[] = [];

const processQueue = (error: unknown, token: string | null = null) => {
  failedQueue.forEach((prom) => {
    if (error) {
      prom.reject(error);
    } else {
      prom.resolve(token);
    }
  });
  failedQueue = [];
};

// Response interceptor for token refresh
api.interceptors.response.use(
  (response) => response,
  async (error) => {
    const originalRequest = error.config;
    if (!error.response) {
      return Promise.reject(error);
    }

    // Handle 401 Unauthorized errors and trigger refresh
    if (error.response.status === 401 && !originalRequest._retry) {
      if (originalRequest.url === '/auth/token' || originalRequest.url === '/auth/login') {
        clearAuthData();
        return Promise.reject(error);
      }

      if (isRefreshing) {
        return new Promise((resolve, reject) => {
          failedQueue.push({ resolve, reject });
        })
          .then((token) => {
            originalRequest.headers.Authorization = `Bearer ${token}`;
            return api(originalRequest);
          })
          .catch((err) => {
            return Promise.reject(err);
          });
      }

      originalRequest._retry = true;
      const refreshToken = getRefreshToken();

      if (!refreshToken) {
        clearAuthData();
        return Promise.reject(error);
      }

      isRefreshing = true;

      try {
        const res = await axios.post<AuthResponse>(`${API_URL}/auth/token`, {
          refreshToken,
        });

        if (res.status === 200) {
          setAuthData(res.data);
          api.defaults.headers.common['Authorization'] = `Bearer ${res.data.accessToken}`;
          processQueue(null, res.data.accessToken);
          originalRequest.headers.Authorization = `Bearer ${res.data.accessToken}`;
          return api(originalRequest);
        }
      } catch (refreshError) {
        processQueue(refreshError, null);
        clearAuthData();
        if (typeof window !== 'undefined') {
          window.location.href = '/auth/login?expired=true';
        }
        return Promise.reject(refreshError);
      } finally {
        isRefreshing = false;
      }
    }

    // Format control plane error response
    const errData = error.response.data;
    if (errData && errData.detail) {
      error.message = errData.detail;
    }
    return Promise.reject(error);
  }
);

// Same page size as the scrolled lists. The loop below keeps going until it has
// everything, so this only trades response size against number of round trips.
const FETCH_ALL_PAGE = LIST_PAGE_SIZE;

/**
 * Reads every page of a list endpoint. For lists that feed a <Select> or a lookup,
 * where a partial list silently gives the wrong answer instead of a shorter one.
 */
export async function fetchAll<T>(
  path: string,
  params?: Record<string, string | number | boolean | undefined>
): Promise<T[]> {
  const items: T[] = [];
  for (;;) {
    const res = await api.get<ListResponse<T>>(path, {
      params: { ...params, limit: FETCH_ALL_PAGE, offset: items.length },
    });
    const page = res.data.items ?? [];
    items.push(...page);
    // Guard on an empty page too — without it a shrinking collection loops forever.
    if (page.length === 0 || items.length >= res.data.numberMatched) return items;
  }
}
