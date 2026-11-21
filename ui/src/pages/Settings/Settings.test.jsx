/* eslint-env jest */
import { vi } from 'vitest';
/* eslint-disable react/prop-types */
import React from 'react';
import { Settings } from './Settings';

vi.mock('rxjs', () => ({
  interval: vi.fn(() => ({
    subscribe: vi.fn(() => ({
      unsubscribe: vi.fn(),
    })),
  })),
}));

vi.mock('@kerberos-io/ui', () => {
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

vi.mock('react-redux', () => ({
  connect: () => (Component) => Component,
}));
vi.mock('react-i18next', () => ({
  withTranslation: () => (Component) => Component,
}));
vi.mock('react-router-dom', () => ({
  Link: ({ children }) => <>{children}</>,
  withRouter: (Component) => Component,
}));
vi.mock('@giantmachines/redux-websocket', () => ({
  send: vi.fn(),
}));
vi.mock('../../components/ImageCanvas/ImageCanvas', () => ({
  default: () => null,
}));
vi.mock('../../websocket', () => ({
  default: 'uuid-1',
}));
vi.mock('../../actions/agent', () => ({
  addRegion: vi.fn(),
  updateRegion: vi.fn(),
  removeRegion: vi.fn(),
  saveConfig: vi.fn(),
  verifyOnvif: vi.fn(),
  verifyCamera: vi.fn(),
  verifyHub: vi.fn(),
  verifyPersistence: vi.fn(),
  verifySecondaryPersistence: vi.fn(),
  getConfig: vi.fn(),
  updateConfig: vi.fn(),
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
    dispatchVerifyHub: vi.fn(),
    dispatchVerifyPersistence: vi.fn(),
    dispatchVerifySecondaryPersistence: vi.fn(),
    dispatchGetConfig: vi.fn(),
    dispatchUpdateConfig: vi.fn(),
    dispatchSaveConfig: vi.fn(),
    dispatchAddRegion: vi.fn(),
    dispatchUpdateRegion: vi.fn(),
    dispatchRemoveRegion: vi.fn(),
    dispatchVerifyCamera: vi.fn(),
    dispatchVerifyOnvif: vi.fn(),
    dispatchSend: vi.fn(),
  });

  beforeEach(() => {
    vi.clearAllMocks();
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
    settings.initialiseLiveview = vi.fn();
    settings.stopLiveview = vi.fn();

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
    settings.requestSDLiveviewFrame = vi.fn();

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
      unsubscribe: vi.fn(),
    };

    settings.componentWillUnmount();

    expect(settings.requestStreamSubscription).toBeNull();
    expect(props.dispatchSend).toHaveBeenCalledWith({
      client_id: 'uuid-1',
      message_type: 'stop-sd',
    });
  });
});
