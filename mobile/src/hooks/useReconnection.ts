import { useCallback, useRef, useState } from 'react';
import type { ReconnectionState } from '../types/signaling';

export interface UseReconnectionConfig {
  maxAttempts: number;
  baseDelay: number; // ms
  onReconnect: () => Promise<void>;
  onFailed: () => void;
}

export interface UseReconnectionReturn {
  state: ReconnectionState;
  attempt: number;
  trigger: () => void;
  cancel: () => void;
  reset: () => void;
}

export function useReconnection(
  config: UseReconnectionConfig
): UseReconnectionReturn {
  const [state, setState] = useState<ReconnectionState>('connected');
  const [attempt, setAttempt] = useState(0);
  const cancelledRef = useRef(false);
  const configRef = useRef(config);
  configRef.current = config;

  const trigger = useCallback(() => {
    // If cancelled (user-initiated leave), do not reconnect
    if (cancelledRef.current) return;

    const { maxAttempts, baseDelay, onReconnect, onFailed } = configRef.current;

    setState('reconnecting');

    const attemptConnect = (currentAttempt: number) => {
      if (cancelledRef.current) return;
      if (currentAttempt > maxAttempts) {
        setState('failed');
        onFailed();
        return;
      }

      setAttempt(currentAttempt);

      // Exponential backoff: baseDelay * 2^(attempt - 1)
      const delay = baseDelay * Math.pow(2, currentAttempt - 1);

      setTimeout(() => {
        if (cancelledRef.current) return;

        onReconnect()
          .then(() => {
            // Success — reset is called by the consumer after confirming connection
            setAttempt(0);
            setState('connected');
          })
          .catch(() => {
            attemptConnect(currentAttempt + 1);
          });
      }, delay);
    };

    attemptConnect(1);
  }, []);

  const cancel = useCallback(() => {
    cancelledRef.current = true;
    setState('connected');
    setAttempt(0);
  }, []);

  const reset = useCallback(() => {
    cancelledRef.current = false;
    setState('connected');
    setAttempt(0);
  }, []);

  return { state, attempt, trigger, cancel, reset };
}
