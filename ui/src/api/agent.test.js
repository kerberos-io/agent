/* eslint-env jest */
import API from './api';
import { doGetDashboardInformation } from './agent';

jest.mock('./api', () => ({
  get: jest.fn(),
}));

describe('dashboard API', () => {
  it('bounds dashboard requests so polling can recover', async () => {
    const onSuccess = jest.fn();
    const onError = jest.fn();
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
