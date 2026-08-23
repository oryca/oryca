'use client';

import { useCallback } from 'react';
import { useQuery } from '@tanstack/react-query';
import { fetchAll } from './api';
import { useAuth } from '@/app/providers';

/** One row of GET /packages/:id/services — the services a package was granted. */
interface PackageSvcLink {
  id: string;
  service?: { id: string };
}

/** Anything the scope can filter. isPublic is optional so trimmed service shapes fit too. */
interface ScopableService {
  id: string;
  isPublic?: boolean;
}

/**
 * GET /services hands the whole catalog to every role, so scope it in the client.
 * Admin and root stay unfiltered. A user gets their own package's services plus the public
 * ones, which the gateway lets anyone call regardless of package.
 */
export function usePackageServiceScope() {
  const { user } = useAuth();
  const isScoped = user?.role === 'user';
  const hasPackage = !!user?.packageId;

  const {
    data: grantedIds,
    isLoading,
    isError,
    refetch,
  } = useQuery<Set<string>>({
    queryKey: ['package-service-ids', user?.packageId],
    queryFn: async () => {
      const links = await fetchAll<PackageSvcLink>(`/packages/${user!.packageId}/services`);
      return new Set(links.map((l) => l.service?.id).filter((id): id is string => !!id));
    },
    enabled: isScoped && hasPackage,
  });

  const filterServices = useCallback(
    <T extends ScopableService>(list: T[] | undefined): T[] => {
      if (!list) return [];
      if (!isScoped) return list;
      // Grants still loading, or the user has no package at all — show nothing rather
      // than briefly flashing the full catalog.
      if (!grantedIds) return [];
      return list.filter((svc) => svc.isPublic || grantedIds.has(svc.id));
    },
    [isScoped, grantedIds]
  );

  return {
    isScoped,
    isLoadingScope: isScoped && hasPackage && isLoading,
    /* The grants are in hand, so an empty result means the package really is empty. While this
     * is false the filter has nothing to match against and returns [] whatever the catalog holds
     * — callers must not read that as "no services". */
    isScopeReady: !isScoped || !!grantedIds,
    /** the fetch failed, as opposed to the user simply having no package */
    isScopeError: isScoped && hasPackage && isError,
    /** a scoped user with nothing to fetch grants from — every non-public service is out of reach */
    hasNoPackage: isScoped && !hasPackage,
    retryScope: refetch,
    filterServices,
  };
}
