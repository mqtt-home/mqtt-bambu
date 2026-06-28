import { useEffect, useRef, useState, useCallback } from 'react';
import type { PrinterStatus, SSEEvent } from '@/types/status';
import { API_BASE } from '@/lib/api';

interface SSEHookReturn {
  statuses: Record<string, PrinterStatus>;
  availabilities: Record<string, boolean>;
  isConnected: boolean;
  error: string | null;
  reconnect: () => void;
}

export function useSSE(): SSEHookReturn {
  const [statuses, setStatuses] = useState<Record<string, PrinterStatus>>({});
  const [availabilities, setAvailabilities] = useState<Record<string, boolean>>({});
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const eventSourceRef = useRef<EventSource | null>(null);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const cleanup = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
      eventSourceRef.current = null;
    }
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }
  }, []);

  const connect = useCallback(() => {
    cleanup();

    try {
      const eventSource = new EventSource(`${API_BASE}/events`);
      eventSourceRef.current = eventSource;

      eventSource.onopen = () => {
        setIsConnected(true);
        setError(null);
      };

      eventSource.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data) as SSEEvent;
          if (data.type === 'availability') {
            setAvailabilities(prev => ({ ...prev, [data.slug]: data.online }));
          } else if (data.type === 'status') {
            const { type, slug, ...status } = data;
            void type;
            setStatuses(prev => ({ ...prev, [slug]: status as PrinterStatus }));
          }
        } catch {
          setError('Failed to parse server data');
        }
      };

      eventSource.onerror = () => {
        setIsConnected(false);
        setError(eventSource.readyState === EventSource.CLOSED
          ? 'Connection closed by server'
          : 'Connection error');

        reconnectTimeoutRef.current = setTimeout(() => {
          if (eventSourceRef.current === eventSource) {
            connect();
          }
        }, 3000);
      };
    } catch {
      setError('Failed to connect to server');
    }
  }, [cleanup]);

  useEffect(() => {
    connect();
    return cleanup;
  }, [connect, cleanup]);

  const reconnect = useCallback(() => {
    setError(null);
    connect();
  }, [connect]);

  return { statuses, availabilities, isConnected, error, reconnect };
}
