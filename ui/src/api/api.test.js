/* eslint-env jest */
import API from './api';
import config from '../config';

jest.mock('../config', () => ({
  API_URL: 'http://localhost/api',
}));

describe('HTTP client compatibility', () => {
  let originalAdapter;
  let adapter;

  beforeEach(() => {
    originalAdapter = API.defaults.adapter;
    adapter = jest.fn((request) =>
      Promise.resolve({
        config: request,
        status: 200,
        statusText: 'OK',
        headers: {},
        data: '{"ok":true}',
      })
    );
    API.defaults.adapter = adapter;
    localStorage.setItem('token', 'test-token');
  });

  afterEach(() => {
    API.defaults.adapter = originalAdapter;
    localStorage.clear();
  });

  it('preserves API URLs, bearer authentication, timeouts and JSON responses', async () => {
    const response = await API.get('dashboard', { timeout: 15000 });
    const request = adapter.mock.calls[0][0];

    expect(request.baseURL).toBe(config.API_URL);
    expect(request.url).toBe('dashboard');
    expect(request.method).toBe('get');
    expect(request.headers.Authorization).toBe('Bearer test-token');
    expect(request.timeout).toBe(15000);
    expect(response.data).toEqual({ ok: true });
  });

  it('serializes saved configuration as JSON and reads refreshed credentials', async () => {
    localStorage.setItem('token', 'refreshed-token');
    const settings = { capture: { ipcamera: { rtsp: 'rtsp://camera/live' } } };

    await API.post('config', settings);
    const request = adapter.mock.calls[0][0];

    expect(request.method).toBe('post');
    expect(request.headers.Authorization).toBe('Bearer refreshed-token');
    expect(request.headers['Content-Type']).toMatch(/^application\/json/);
    expect(JSON.parse(request.data)).toEqual(settings);
  });

  it('preserves rejected requests for API error handlers', async () => {
    const error = new Error('Request timed out');
    adapter.mockRejectedValueOnce(error);

    await expect(API.get('dashboard', { timeout: 15000 })).rejects.toBe(error);
  });
});
