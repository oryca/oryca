'use client';

import { useEffect, useState } from 'react';

/**
 * Trails `value` by `delay`. Search boxes feed this into a query param so a
 * keystroke isn't a request.
 */
export function useDebounced<T>(value: T, delay = 300): T {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(id);
  }, [value, delay]);

  return debounced;
}
