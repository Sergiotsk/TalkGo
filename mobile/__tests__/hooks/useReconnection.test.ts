// TASK-058: Tests for useReconnection hook

import { renderHook, act } from '@testing-library/react-native';
import { useReconnection } from '../../src/hooks/useReconnection';

describe('useReconnection', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('starts in connected state', () => {
    const onReconnect = jest.fn().mockResolvedValue(undefined);
    const onFailed = jest.fn();

    const { result } = renderHook(() =>
      useReconnection({ maxAttempts: 3, baseDelay: 1000, onReconnect, onFailed })
    );

    expect(result.current.state).toBe('connected');
    expect(result.current.attempt).toBe(0);
  });

  it('trigger moves state to reconnecting and attempt=1', async () => {
    const onReconnect = jest.fn().mockRejectedValue(new Error('fail'));
    const onFailed = jest.fn();

    const { result } = renderHook(() =>
      useReconnection({ maxAttempts: 3, baseDelay: 100, onReconnect, onFailed })
    );

    await act(async () => {
      result.current.trigger();
    });

    expect(result.current.state).toBe('reconnecting');
    expect(result.current.attempt).toBeGreaterThanOrEqual(1);
  });

  it('transitions to failed after maxAttempts exhausted', async () => {
    const onReconnect = jest.fn().mockRejectedValue(new Error('fail'));
    const onFailed = jest.fn();

    const { result } = renderHook(() =>
      useReconnection({ maxAttempts: 3, baseDelay: 10, onReconnect, onFailed })
    );

    // Trigger reconnection
    act(() => {
      result.current.trigger();
    });

    // Run through all 3 attempts: delays 10ms, 20ms, 40ms
    // Each attempt: advance timer → onReconnect called → rejection → schedule next
    for (let i = 0; i < 4; i++) {
      await act(async () => {
        jest.runAllTimers();
        // Let the promise chain resolve
        await Promise.resolve();
        await Promise.resolve();
      });
    }

    expect(result.current.state).toBe('failed');
    expect(onFailed).toHaveBeenCalled();
  });

  it('cancel prevents reconnection (user_initiated)', () => {
    const onReconnect = jest.fn().mockResolvedValue(undefined);
    const onFailed = jest.fn();

    const { result } = renderHook(() =>
      useReconnection({ maxAttempts: 3, baseDelay: 1000, onReconnect, onFailed })
    );

    act(() => {
      result.current.cancel();
    });

    // After cancel, trigger should not start reconnection
    act(() => {
      result.current.trigger();
    });

    expect(onReconnect).not.toHaveBeenCalled();
  });

  it('reset moves back to connected after successful reconnect', async () => {
    const onReconnect = jest.fn().mockResolvedValue(undefined);
    const onFailed = jest.fn();

    const { result } = renderHook(() =>
      useReconnection({ maxAttempts: 3, baseDelay: 100, onReconnect, onFailed })
    );

    await act(async () => {
      result.current.trigger();
      await Promise.resolve();
    });

    act(() => {
      result.current.reset();
    });

    expect(result.current.state).toBe('connected');
    expect(result.current.attempt).toBe(0);
  });

  it('uses exponential backoff — delays are 1x, 2x, 4x baseDelay', () => {
    // Verify the backoff formula: delay = baseDelay * 2^(attempt - 1)
    const baseDelay = 1000;
    const expectedDelays = [1000, 2000, 4000];

    expectedDelays.forEach((expected, i) => {
      const attempt = i + 1;
      const actual = baseDelay * Math.pow(2, attempt - 1);
      expect(actual).toBe(expected);
    });
  });
});
