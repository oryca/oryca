/* Hallmark · component: infinite-scroll sentinel · genre: modern-minimal · theme: Cobalt
 * Sits at the end of a paged list and pulls the next page once it scrolls into view.
 */
'use client';

import React, { useEffect, useRef } from 'react';
import { Button } from './Button';
import { SkeletonLine } from './Skeleton';

export interface InfiniteScrollSentinelProps {
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  fetchNextPage: () => void;
  /** The scrolling ancestor to watch. Omit to watch the viewport. */
  rootRef?: React.RefObject<HTMLElement | null>;
  /** Fires the next page this far before the sentinel is actually on screen. */
  rootMargin?: string;
  label?: string;
}

export function InfiniteScrollSentinel({
  hasNextPage,
  isFetchingNextPage,
  fetchNextPage,
  rootRef,
  rootMargin = '200px',
  label = 'Load more',
}: InfiniteScrollSentinelProps) {
  const ref = useRef<HTMLDivElement>(null);
  // Read through a ref so re-creating fetchNextPage each render doesn't rebuild the observer.
  const fetchRef = useRef(fetchNextPage);
  useEffect(() => {
    fetchRef.current = fetchNextPage;
  }, [fetchNextPage]);

  useEffect(() => {
    const el = ref.current;
    if (!el || !hasNextPage || isFetchingNextPage) return;
    if (typeof IntersectionObserver === 'undefined') return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) fetchRef.current();
      },
      // The container's ref is already attached by the time this child effect runs.
      { root: rootRef?.current ?? null, rootMargin }
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [hasNextPage, isFetchingNextPage, rootMargin, rootRef]);

  if (!hasNextPage && !isFetchingNextPage) return null;

  return (
    <div ref={ref} className="py-4">
      {isFetchingNextPage ? (
        <div role="status" aria-live="polite" aria-label="Loading more">
          <div className="space-y-2 px-4">
            <SkeletonLine width="100%" />
            <SkeletonLine width="72%" />
          </div>
        </div>
      ) : (
        // The observer normally gets here first; this keeps the list reachable
        // by keyboard and when IntersectionObserver is unavailable.
        <div className="flex justify-center">
          <Button variant="ghost" size="sm" onClick={() => fetchNextPage()}>
            {label}
          </Button>
        </div>
      )}
    </div>
  );
}
