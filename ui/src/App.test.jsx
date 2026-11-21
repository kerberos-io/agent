/* eslint-env jest */
import { vi } from 'vitest';
/* eslint-disable react/prop-types */
import React from 'react';
import { interval } from 'rxjs';
import { App } from './App';

const subscriptions = [];

vi.mock('rxjs', () => ({
  interval: vi.fn(),
}));

vi.mock('@kerberos-io/ui', () => {
  const Mock = ({ children }) => <div>{children}</div>;
  return {
    Badge: Mock,
    Main: Mock,
    MainBody: Mock,
    Gradient: Mock,
    Sidebar: Mock,
    Navigation: Mock,
    NavigationSection: Mock,
    NavigationItem: Mock,
    NavigationGroup: Mock,
    Profilebar: Mock,
    Icon: Mock,
  };
});

vi.mock('./components/LanguageSelect/LanguageSelect', () => ({
  default: () => null,
}));
vi.mock('./actions', () => ({ logout: vi.fn() }));
vi.mock('./actions/agent', () => ({
  getDashboardInformation: vi.fn(),
}));
vi.mock('./config', () => ({
  default: {
    MODE: 'release',
    URL: 'http://localhost',
    WS_URL: 'ws://localhost/ws',
  },
}));
vi.mock('react-redux', () => ({
  connect: () => (Component) => Component,
}));
vi.mock('react-i18next', () => ({
  withTranslation: () => (Component) => Component,
}));
vi.mock('react-router-dom', () => ({
  Link: ({ children }) => <>{children}</>,
}));
vi.mock('@giantmachines/redux-websocket', () => ({
  connect: vi.fn(),
  disconnect: vi.fn(),
  send: vi.fn(),
}));
vi.mock('uuid', () => ({
  v4: vi.fn(() => 'uuid-1'),
}));

describe('App lifecycle refresh behaviour', () => {
  const baseProps = () => ({
    t: (value) => value,
    dispatchLogout: vi.fn(),
    dispatchConnect: vi.fn(),
    dispatchDisconnect: vi.fn(),
    dispatchSend: vi.fn(),
    dispatchGetDashboardInformation: vi.fn((onSuccess) => {
      if (onSuccess) {
        onSuccess();
      }
    }),
    children: [],
    username: 'admin',
    connected: false,
    dashboard: {
      cloudOnline: 0,
      offlineMode: 'false',
    },
  });

  beforeEach(() => {
    subscriptions.length = 0;
    vi.clearAllMocks();
    interval.mockImplementation(() => ({
      subscribe: vi.fn((callback) => {
        const subscription = {
          callback,
          unsubscribe: vi.fn(),
        };
        subscriptions.push(subscription);
        return subscription;
      }),
    }));
    Object.defineProperty(document, 'hidden', {
      configurable: true,
      writable: true,
      value: false,
    });
  });

  it('uses a single dashboard interval and skips hidden or in-flight refreshes', () => {
    const props = baseProps();
    const app = new App(props);
    app.props = props;

    app.componentDidMount();

    expect(props.dispatchConnect).toHaveBeenCalledTimes(1);
    expect(props.dispatchGetDashboardInformation).toHaveBeenCalledTimes(1);
    expect(interval).toHaveBeenCalledWith(5000);
    expect(subscriptions).toHaveLength(1);

    document.hidden = true;
    subscriptions[0].callback();
    expect(props.dispatchGetDashboardInformation).toHaveBeenCalledTimes(1);

    document.hidden = false;
    app.dashboardRequestInFlight = true;
    subscriptions[0].callback();
    expect(props.dispatchGetDashboardInformation).toHaveBeenCalledTimes(1);

    app.dashboardRequestInFlight = false;
    subscriptions[0].callback();
    expect(props.dispatchGetDashboardInformation).toHaveBeenCalledTimes(2);
  });

  it('sends hello after reconnect without issuing a second manual reconnect', () => {
    const props = {
      ...baseProps(),
      connected: true,
    };
    const app = new App(props);
    app.props = props;

    app.componentDidUpdate({
      ...props,
      connected: false,
    });
    expect(props.dispatchSend).toHaveBeenCalledWith(
      expect.objectContaining({
        client_id: 'uuid-1',
        message_type: 'hello',
      })
    );

    props.dispatchSend.mockClear();
    props.dispatchConnect.mockClear();
    app.props = {
      ...props,
      connected: false,
    };
    app.componentDidUpdate({
      ...props,
      connected: true,
    });

    expect(props.dispatchSend).not.toHaveBeenCalled();
    expect(props.dispatchConnect).not.toHaveBeenCalled();
  });
});
