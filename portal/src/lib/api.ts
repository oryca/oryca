import axios from 'axios';

export const NEXT_PUBLIC_API_URL =
  process.env.NEXT_PUBLIC_API_URL || 'http://localhost:9001/control-plane/api/v1';

export const NEXT_PUBLIC_GATEWAY_URL =
  process.env.NEXT_PUBLIC_GATEWAY_URL || 'http://localhost:9002/gateway/api';

export interface UserSession {
  id: string;
  email: string;
  role: 'user' | 'admin' | 'root';
  firstName?: string;
  lastName?: string;
  displayName?: string;
  packageId?: string;
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
  baseURL: NEXT_PUBLIC_API_URL,
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
let failedQueue: any[] = [];

const processQueue = (error: any, token: string | null = null) => {
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
        const res = await axios.post<AuthResponse>(`${NEXT_PUBLIC_API_URL}/auth/token`, {
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
