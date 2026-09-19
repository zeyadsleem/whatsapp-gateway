import { useCallback, useEffect, useRef, useState } from 'react';

export interface PollingState<T> {
  data: T | null;
  error: string | null;
  loading: boolean;
  refresh: () => Promise<void>;
}

export function usePolling<T>(fn: () => Promise<T>, intervalMs: number): PollingState<T> {
  const fnRef = useRef(fn);
  const mountedRef = useRef(true);
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fnRef.current = fn;
  });

  const refresh = useCallback(async (): Promise<void> => {
    try {
      const result = await fnRef.current();
      if (!mountedRef.current) {
        return;
      }
      setData(result);
      setError(null);
    } catch (err) {
      if (!mountedRef.current) {
        return;
      }
      setError(err instanceof Error ? err.message : 'حدث خطأ غير متوقع');
    } finally {
      if (mountedRef.current) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    let timer: number | null = null;

    const stop = (): void => {
      if (timer !== null) {
        window.clearInterval(timer);
        timer = null;
      }
    };

    const start = (): void => {
      if (timer !== null) {
        return;
      }
      void refresh();
      timer = window.setInterval(() => {
        void refresh();
      }, intervalMs);
    };

    const handleVisibilityChange = (): void => {
      if (document.hidden) {
        stop();
      } else {
        start();
      }
    };

    start();
    document.addEventListener('visibilitychange', handleVisibilityChange);

    return () => {
      mountedRef.current = false;
      stop();
      document.removeEventListener('visibilitychange', handleVisibilityChange);
    };
  }, [intervalMs, refresh]);

  return { data, error, loading, refresh };
}
