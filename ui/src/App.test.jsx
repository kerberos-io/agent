/* eslint-env jest */
/* eslint-disable react/prop-types */
import React from 'react';
import { interval } from 'rxjs';
import { App } from './App';

const subscriptions = [];

jest.mock('rxjs', () => ({
  interval: jest.fn(),
}));

jest.mock('@kerberos-io/ui', () => {
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

jest.mock('./components/LanguageSelect/LanguageSelect', () => () => null);
jest.mock('./actions', () => ({ logout: jest.fn() }));
jest.mock('./actions/agent', () => ({
  getDashboardInformation: jest.fn(),
}));
jest.mock('./config', () => ({
  MODE: 'release',
  URL: 'http://localhost',
  WS_URL: 'ws://localhost/ws',
}));
jest.mock('react-redux', () => ({
  connect: () => (Component) => Component,
}));
jest.mock('react-i18next', () => ({
  withTranslation: () => (Component) => Component,
}));
jest.mock('react-router-dom', () => ({
  Link: ({ children }) => <>{children}</>,
}));
jest.mock('@giantmachines/redux-websocket', () => ({
  connect: jest.fn(),
  disconnect: jest.fn(),
  send: jest.fn(),
}));
jest.mock('uuidv4', () => ({
  __esModule: true,
  default: jest.fn(() => 'uuid-1'),
}));

describe('App lifecycle refresh behaviour', () => {
  const baseProps = () => ({
    t: (value) => value,
    dispatchLogout: jest.fn(),
    dispatchConnect: jest.fn(),
    dispatchDisconnect: jest.fn(),
    dispatchSend: jest.fn(),
    dispatchGetDashboardInformation: jest.fn((onSuccess) => {
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
    jest.clearAllMocks();
    interval.mockImplementation(() => ({
      subscribe: jest.fn((callback) => {
        const subscription = {
          callback,
          unsubscribe: jest.fn(),
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
