import React from 'react';
import PropTypes from 'prop-types';
import {
  Main,
  MainBody,
  Gradient,
  Sidebar,
  Navigation,
  NavigationSection,
  NavigationItem,
  NavigationGroup,
  Profilebar,
  Badge,
} from '@kerberos-io/ui';
import {
  connect as connectWS,
  disconnect as disconnectWS,
  send,
} from '@giantmachines/redux-websocket';
import { connect } from 'react-redux';
import './App.module.scss';
import logo from './header-minimal-logo-36x36.svg';
import '@kerberos-io/ui/lib/index.css';
import { logout } from './actions';
import config from './config';

class App extends React.Component {
  componentDidMount() {
    const { dispatchConnect } = this.props;
    dispatchConnect();
  }

  componentDidUpdate(prevProps) {
    // We are connected again, lets fire the initial events.
    const { connected, dispatchConnect, dispatchSend } = this.props;

    if (prevProps.connected === false && connected === true) {
      const message = {
        client_id: 'ok',
        message_type: 'start-watch',
      };
      dispatchSend(message);
    }

    // We disconnected, let's try to connect again
    if (prevProps.connected === true && connected === false) {
      dispatchConnect();
    }
  }

  componentWillUnmount() {
    const message = {
      client_id: 'ok',
      message_type: 'stop-watch',
    };
    const { dispatchSend, dispatchDisconnect } = this.props;
    dispatchSend(message);
    dispatchDisconnect();
  }

  render() {
    const { children, connected, username, dispatchLogout } = this.props;
    return (
      <div id="page-root">
        <Sidebar
          logo={logo}
          title="Kerberos Factory"
          version={config.VERSION}
          mobile
        >
          <Profilebar
            username={username}
            email="support@kerberos.io"
            userrole="admin"
            logout={dispatchLogout}
          />
          <Navigation>
            {/* <NavigationSection title={"monitoring"} />
              <NavigationGroup>
                <NavigationItem
                    title={"Dashboard"}
                    icon={"dashboard"}
                    link={"dashboard"}
                />
              </NavigationGroup> */}
            <NavigationSection title="management" />
            <NavigationGroup>
              <NavigationItem title="Settings" icon="api" link="settings" />
              <NavigationItem
                title="Cameras"
                icon="cameras"
                link="deployments"
              />
              <NavigationItem title="Nodes" icon="counting" link="nodes" />
              <NavigationItem title="Pods" icon="counting" link="pods" />
            </NavigationGroup>
            <NavigationSection title="help & support" />
            <NavigationGroup>
              <NavigationItem
                title="Swagger API docs"
                icon="api"
                external
                link={`${config.API_URL}swagger/index.html`}
              />
              <NavigationItem
                title="Documentation"
                icon="book"
                external
                link="https://doc.kerberos.io/factory/first-things-first"
              />
              <NavigationItem
                title="Github"
                icon="github-nav"
                external
                link="https://github.com/kerberos-io/factory"
              />
            </NavigationGroup>
            <NavigationSection title="API connection" />
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
          <MainBody>{children}</MainBody>
        </Main>
      </div>
    );
  }
}

const mapStateToProps = (state) => ({
  username: state.auth.username,
  connected: state.wss.connected,
});

const mapDispatchToProps = (dispatch) => ({
  dispatchLogout: () => dispatch(logout()),
  dispatchConnect: () => dispatch(connectWS(config.WS_URL)),
  dispatchDisconnect: () => dispatch(disconnectWS()),
  dispatchSend: (message) => dispatch(send(message)),
});

App.propTypes = {
  dispatchLogout: PropTypes.func.isRequired,
  dispatchConnect: PropTypes.func.isRequired,
  dispatchDisconnect: PropTypes.func.isRequired,
  dispatchSend: PropTypes.func.isRequired,
  children: PropTypes.func.isRequired,
  username: PropTypes.func.isRequired,
  connected: PropTypes.func.isRequired,
};

export default connect(mapStateToProps, mapDispatchToProps)(App);
