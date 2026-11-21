/* eslint-env jest */
import { vi } from 'vitest';
import API from './api';
import { doGetDashboardInformation } from './agent';

vi.mock('./api', () => ({
  default: {
    get: vi.fn(),
  },
}));

describe('dashboard API', () => {
  it('bounds dashboard requests so polling can recover', async () => {
    const onSuccess = vi.fn();
    const onError = vi.fn();
    API.get.mockResolvedValue({
      status: 200,
      data: { uptime: '1 minute' },
    });

    doGetDashboardInformation(onSuccess, onError);

    expect(API.get).toHaveBeenCalledWith('dashboard', {
      timeout: 15000,
    });
    await Promise.resolve();
    await Promise.resolve();
    expect(onSuccess).toHaveBeenCalledWith({ uptime: '1 minute' });
    expect(onError).not.toHaveBeenCalled();
  });
});
