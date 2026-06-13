import { renderHook } from '@testing-library/react-native';
import { useKeepAwake as useExpoKeepAwake } from 'expo-keep-awake';
import { useKeepAwake } from '../../src/hooks/useKeepAwake';

jest.mock('expo-keep-awake');

describe('useKeepAwake', () => {
  it('calls useKeepAwake from expo-keep-awake when active', () => {
    renderHook(() => useKeepAwake(true));
    expect(useExpoKeepAwake).toHaveBeenCalled();
  });

  it('does not call useKeepAwake when inactive', () => {
    (useExpoKeepAwake as jest.Mock).mockClear();
    renderHook(() => useKeepAwake(false));
    expect(useExpoKeepAwake).not.toHaveBeenCalled();
  });
});
