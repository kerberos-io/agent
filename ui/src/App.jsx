import React from 'react';
import PropTypes from 'prop-types';
import { withTranslation } from 'react-i18next';
import {
  connect as connectWS,
  disconnect as disconnectWS,
  send,
} from '@giantmachines/redux-websocket';
import {
  Badge,
  Main,
  MainBody,
  Gradient,
  Sidebar,
  Navigation,
  NavigationSection,
  NavigationItem,
  NavigationGroup,
  Profilebar,
  Icon,
} from '@kerberos-io/ui';
import { interval } from 'rxjs';
import { connect } from 'react-redux';
import { Link } from 'react-router-dom';
import { logout } from './actions';
import config from './config';
import { getDashboardInformation } from './actions/agent';
import LanguageSelect from './components/LanguageSelect/LanguageSelect';
import websocketClientId from './websocket';
import logo from './header-minimal-logo-36x36.svg';
import '@kerberos-io/ui/lib/index.css';
import './App.scss';

// eslint-disable-next-line react/prefer-stateless-function
export class App extends React.Component {
  componentDidMount() {
    const { dispatchConnect } = this.props;
    this.dashboardRequestInFlight = false;
    this.handleVisibilityChange = this.handleVisibilityChange.bind(this);
    dispatchConnect();
    this.refreshDashboardInformation(true);

    document.addEventListener(
      'visibilitychange',
      this.handleVisibilityChange,
      false
    );

    const interval$ = interval(5000);
    this.subscription = interval$.subscribe(() => {
      this.refreshDashboardInformation();
    });
  }

  componentDidUpdate(prevProps) {
    // We are connected again, lets fire the initial events.
    const { connected, dispatchSend } = this.props;
    const { connected: connectedPrev } = prevProps;
    if (connectedPrev === false && connected === true) {
      const message = {
        client_id: websocketClientId,
        message_type: 'hello',
      };
      dispatchSend(message);
    }
  }

  componentWillUnmount() {
    this.dashboardRequestInFlight = false;
    document.removeEventListener(
      'visibilitychange',
      this.handleVisibilityChange,
      false
    );

    if (this.subscription) {
      this.subscription.unsubscribe();
      this.subscription = null;
    }

    const message = {
      client_id: websocketClientId,
      message_type: 'goodbye',
    };
    const { connected, dispatchSend, dispatchDisconnect } = this.props;
    if (connected) {
      dispatchSend(message);
    }
    dispatchDisconnect();
  }

  handleVisibilityChange() {
    if (!this.isDocumentHidden()) {
      this.refreshDashboardInformation();
    }
  }

  getCurrentTimestamp() {
    return Math.round(Date.now() / 1000);
  }

  isDocumentHidden() {
    return typeof document !== 'undefined' && document.hidden;
  }

  refreshDashboardInformation(force = false) {
    if (this.dashboardRequestInFlight) {
      return;
    }

    if (!force && this.isDocumentHidden()) {
      return;
    }

    this.dashboardRequestInFlight = true;
    const { dispatchGetDashboardInformation } = this.props;
    dispatchGetDashboardInformation(
      () => {
        this.dashboardRequestInFlight = false;
      },
      (error) => {
        this.dashboardRequestInFlight = false;
        // eslint-disable-next-line no-console
        console.warn('Unable to refresh dashboard information.', error);
      }
    );
  }

  render() {
    const { t, connected } = this.props;
    const { children, username, dashboard, dispatchLogout } = this.props;
    const cloudOnline = this.getCurrentTimestamp() - dashboard.cloudOnline < 30;
    return (
      <>
        {config.MODE !== 'release' && (
          <div className={`environment ${config.MODE}`}>
            Environment: {config.MODE}
          </div>
        )}
        <div id="page-root">
          <Sidebar logo={logo} title="Kerberos Agent" version="v3.1.8" mobile>
            <Profilebar
              username={username}
              email="support@kerberos.io"
              userrole={t('navigation.admin')}
              logout={dispatchLogout}
            />
            <Navigation>
              <NavigationSection title={t('navigation.management')} />
              <NavigationGroup>
                <NavigationItem
                  title={t('navigation.dashboard')}
                  icon="dashboard"
                  link="dashboard"
                />
                <NavigationItem
                  title={t('navigation.recordings')}
                  icon="media"
                  link="media"
                />
                <NavigationItem
                  title={t('navigation.settings')}
                  icon="preferences"
                  link="settings"
                />
              </NavigationGroup>
              <NavigationSection title={t('navigation.help_support')} />
              <NavigationGroup>
                <NavigationItem
                  title={t('navigation.swagger')}
                  icon="api"
                  external
                  link={`${config.URL}/swagger/index.html`}
                />
                <NavigationItem
                  title={t('navigation.documentation')}
                  icon="book"
                  external
                  link="https://doc.kerberos.io/agent/announcement"
                />
                <NavigationItem
                  title="Kerberos Hub"
                  icon="cloud"
                  external
                  link="https://app.kerberos.io"
                />
                <NavigationItem
                  title={t('navigation.ui_library')}
                  icon="paint"
                  external
                  link="https://ui.kerberos.io/"
                />
                <NavigationItem
                  title="Github"
                  icon="github-nav"
                  external
                  link="https://github.com/kerberos-io/agent"
                />
              </NavigationGroup>
              <NavigationSection title={t('navigation.layout')} />
              <NavigationGroup>
                <LanguageSelect />
              </NavigationGroup>

              <NavigationSection title="Websocket" />
              <NavigationGroup>
                <div className="websocket-badge">
                  <Badge
                    title={connected ? 'connected' : 'disconnected'}
                    status={connected ? 'success' : 'warning'}
                  />
                </div>
              </NavigationGroup>
            </Navigation>
          </Sidebar>
          <Main>
            <Gradient />

            {!cloudOnline && (
              <a
                href="https://app.kerberos.io"
                target="_blank"
                rel="noreferrer"
              >
                <div className="cloud-not-installed">
                  <div>
                    <Icon label="cloud" />
                    Activate Kerberos Hub, and make your cameras and recordings
                    available through a secured cloud!
                  </div>
                </div>
              </a>
            )}

            {dashboard.offlineMode === 'true' && (
              <Link to="/settings">
                <div className="offline-mode">
                  <div>
                    <Icon label="info" />
                    Attention! Kerberos is currently running in Offline mode.
                  </div>
                </div>
              </Link>
            )}

            <MainBody>{children}</MainBody>
          </Main>
        </div>
      </>
    );
  }
}

const mapStateToProps = (state) => ({
  username: state.authentication.username,
  dashboard: state.agent.dashboard,
  connected: state.wss.connected,
});

const mapDispatchToProps = (dispatch) => ({
  dispatchLogout: () => dispatch(logout()),
  dispatchConnect: () => {
    dispatch(connectWS(config.WS_URL));
  },
  dispatchDisconnect: () => dispatch(disconnectWS()),
  dispatchSend: (message) => dispatch(send(message)),
  dispatchGetDashboardInformation: (onSuccess, onError) =>
    dispatch(getDashboardInformation(onSuccess, onError)),
});

App.propTypes = {
  t: PropTypes.func.isRequired,
  dispatchLogout: PropTypes.func.isRequired,
  dispatchConnect: PropTypes.func.isRequired,
  dispatchDisconnect: PropTypes.func.isRequired,
  dispatchSend: PropTypes.func.isRequired,
  // eslint-disable-next-line react/forbid-prop-types
  children: PropTypes.array.isRequired,
  username: PropTypes.string.isRequired,
  connected: PropTypes.bool.isRequired,
  dashboard: PropTypes.object.isRequired,
  dispatchGetDashboardInformation: PropTypes.func.isRequired,
};

export default withTranslation()(
  connect(mapStateToProps, mapDispatchToProps)(App)
);
