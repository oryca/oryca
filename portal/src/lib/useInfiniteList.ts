'use client';

import { useMemo } from 'react';
import { useInfiniteQuery, keepPreviousData } from '@tanstack/react-query';
import { api, LIST_PAGE_SIZE, type ListResponse } from './api';

export { LIST_PAGE_SIZE };

export interface InfiniteListParams {
  [key: string]: string | number | boolean | undefined;
}

/**
 * Pages a control-plane list endpoint by offset, stopping once the loaded rows
 * reach numberMatched. Pair with <InfiniteScrollSentinel> to fetch on scroll.
 *
 * Filtering and search belong in `params` so the server does them — a client-side
 * filter would only ever see the rows already loaded.
 */
export function useInfiniteList<T>(
  queryKey: readonly unknown[],
  path: string,
  params?: InfiniteListParams,
  options?: { enabled?: boolean }
) {
  const query = useInfiniteQuery({
    queryKey: [...queryKey, params ?? null],
    initialPageParam: 0,
    queryFn: async ({ pageParam }) => {
      const res = await api.get<ListResponse<T>>(path, {
        params: { ...params, limit: LIST_PAGE_SIZE, offset: pageParam },
      });
      return res.data;
    },
    getNextPageParam: (lastPage, allPages) => {
      const loaded = allPages.reduce((n, p) => n + (p.items?.length ?? 0), 0);
      // Stop on a short page as well — otherwise a stale numberMatched keeps requesting.
      if (!lastPage.items?.length || loaded >= lastPage.numberMatched) return undefined;
      return loaded;
    },
    // A search keystroke changes the key. Without this the list blanks to skeletons
    // on every debounced change and scroll position is lost.
    placeholderData: keepPreviousData,
    enabled: options?.enabled,
  });

  const items = useMemo(
    () => query.data?.pages.flatMap((p) => p.items ?? []) ?? [],
    [query.data]
  );

  return {
    ...query,
    items: items as T[],
    total: query.data?.pages[0]?.numberMatched ?? 0,
  };
}
