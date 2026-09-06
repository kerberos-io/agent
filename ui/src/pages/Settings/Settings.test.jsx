/* eslint-env jest */
/* eslint-disable react/prop-types */
import React from 'react';
import { Settings } from './Settings';

jest.mock('rxjs', () => ({
  interval: jest.fn(() => ({
    subscribe: jest.fn(() => ({
      unsubscribe: jest.fn(),
    })),
  })),
}));

jest.mock('@kerberos-io/ui', () => {
  const Mock = ({ children }) => <div>{children}</div>;
  return {
    Breadcrumb: Mock,
    ControlBar: Mock,
    Input: Mock,
    Button: Mock,
    Block: Mock,
    BlockHeader: Mock,
    BlockBody: Mock,
    BlockFooter: Mock,
    InfoBar: Mock,
    Dropdown: Mock,
    Tabs: Mock,
    Tab: Mock,
    Icon: Mock,
    Toggle: Mock,
  };
});

jest.mock('react-redux', () => ({
  connect: () => (Component) => Component,
}));
jest.mock('react-i18next', () => ({
  withTranslation: () => (Component) => Component,
}));
jest.mock('react-router-dom', () => ({
  Link: ({ children }) => <>{children}</>,
  withRouter: (Component) => Component,
}));
jest.mock('@giantmachines/redux-websocket', () => ({
  send: jest.fn(),
}));
jest.mock('../../components/ImageCanvas/ImageCanvas', () => () => null);
jest.mock('../../websocket', () => ({
  __esModule: true,
  default: 'uuid-1',
}));
jest.mock('../../actions/agent', () => ({
  addRegion: jest.fn(),
  updateRegion: jest.fn(),
  removeRegion: jest.fn(),
  saveConfig: jest.fn(),
  verifyOnvif: jest.fn(),
  verifyCamera: jest.fn(),
  verifyHub: jest.fn(),
  verifyPersistence: jest.fn(),
  verifySecondaryPersistence: jest.fn(),
  getConfig: jest.fn(),
  updateConfig: jest.fn(),
}));

describe('Settings liveview lifecycle', () => {
  const config = {
    config: {
      offline: 'false',
      region: {
        polygon: [],
      },
    },
  };

  const baseProps = () => ({
    t: (value) => value,
    connected: true,
    config,
    images: [],
    dispatchVerifyHub: jest.fn(),
    dispatchVerifyPersistence: jest.fn(),
    dispatchVerifySecondaryPersistence: jest.fn(),
    dispatchGetConfig: jest.fn(),
    dispatchUpdateConfig: jest.fn(),
    dispatchSaveConfig: jest.fn(),
    dispatchAddRegion: jest.fn(),
    dispatchUpdateRegion: jest.fn(),
    dispatchRemoveRegion: jest.fn(),
    dispatchVerifyCamera: jest.fn(),
    dispatchVerifyOnvif: jest.fn(),
    dispatchSend: jest.fn(),
  });

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('starts and stops SD imagery with the conditions editor visibility', () => {
    const props = baseProps();
    const settings = new Settings(props);
    settings.props = props;
    settings.state = {
      ...settings.state,
      selectedTab: 'conditions',
      search: '',
    };
    settings.initialiseLiveview = jest.fn();
    settings.stopLiveview = jest.fn();

    settings.componentDidUpdate(props, {
      ...settings.state,
      selectedTab: 'overview',
      search: '',
    });
    expect(settings.initialiseLiveview).toHaveBeenCalledTimes(1);

    settings.initialiseLiveview.mockClear();
    settings.state = {
      ...settings.state,
      selectedTab: 'overview',
    };
    settings.componentDidUpdate(props, {
      ...settings.state,
      selectedTab: 'conditions',
      search: '',
    });
    expect(settings.stopLiveview).toHaveBeenCalledTimes(1);
  });

  it('re-requests SD imagery after reconnect when the editor is active', () => {
    const props = baseProps();
    const settings = new Settings(props);
    settings.props = props;
    settings.state = {
      ...settings.state,
      selectedTab: 'conditions',
      search: '',
    };
    settings.sdLiveviewActive = true;
    settings.requestSDLiveviewFrame = jest.fn();

    settings.componentDidUpdate(
      {
        ...props,
        connected: false,
      },
      settings.state
    );

    expect(settings.requestSDLiveviewFrame).toHaveBeenCalledTimes(1);
  });

  it('stops the SD imagery subscription on unmount', () => {
    const props = baseProps();
    const settings = new Settings(props);
    settings.props = props;
    settings.sdLiveviewActive = true;
    settings.requestStreamSubscription = {
      unsubscribe: jest.fn(),
    };

    settings.componentWillUnmount();

    expect(settings.requestStreamSubscription).toBeNull();
    expect(props.dispatchSend).toHaveBeenCalledWith({
      client_id: 'uuid-1',
      message_type: 'stop-sd',
    });
  });
});
