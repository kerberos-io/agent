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
  Icon,
} from '@kerberos-io/ui';
import { interval } from 'rxjs';
import { connect } from 'react-redux';
import logo from './header-minimal-logo-36x36.svg';
import '@kerberos-io/ui/lib/index.css';
import { logout } from './actions';
import config from './config';
import './App.scss';
import { GetDashboardInformation } from './actions/agent';

// eslint-disable-next-line react/prefer-stateless-function
class App extends React.Component {
  componentDidMount() {
    const { dispatchGetDashboardInformation } = this.props;
    dispatchGetDashboardInformation();

    const interval$ = interval(5000);
    this.subscription = interval$.subscribe(() => {
      dispatchGetDashboardInformation();
    });
  }

  componentWillUnmount() {
    this.subscription.unsubscribe();
  }

  render() {
    const { children, username, dashboard, dispatchLogout } = this.props;
    return (
      <div id="page-root">
        <Sidebar
          logo={logo}
          title="Kerberos Agent"
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
            <NavigationSection title="management" />
            <NavigationGroup>
              <NavigationItem
                title="Dashboard"
                icon="dashboard"
                link="dashboard"
              />
              <NavigationItem title="Recordings" icon="media" link="media" />
              <NavigationItem
                title="Settings"
                icon="preferences"
                link="settings"
              />
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
                link="https://doc.kerberos.io/agent/announcement"
              />
              <NavigationItem
                title="Github"
                icon="github-nav"
                external
                link="https://github.com/kerberos-io/agent"
              />
            </NavigationGroup>
          </Navigation>
        </Sidebar>
        <Main>
          <Gradient />

          {dashboard.offlineMode === 'true' && (
            <div className="warning">
              <Icon label="info" />
              Attention! Kerberos is currently running in Offline mode.
            </div>
          )}

          <MainBody>{children}</MainBody>
        </Main>
      </div>
    );
  }
}

const mapStateToProps = (state) => ({
  username: state.authentication.username,
  dashboard: state.agent.dashboard,
});

const mapDispatchToProps = (dispatch) => ({
  dispatchLogout: () => dispatch(logout()),
  dispatchGetDashboardInformation: (dashboard, success, error) =>
    dispatch(GetDashboardInformation(dashboard, success, error)),
});

App.propTypes = {
  dispatchLogout: PropTypes.func.isRequired,
  // eslint-disable-next-line react/forbid-prop-types
  children: PropTypes.array.isRequired,
  username: PropTypes.string.isRequired,
  dashboard: PropTypes.objectOf(PropTypes.object).isRequired,
  dispatchGetDashboardInformation: PropTypes.func.isRequired,
};

export default connect(mapStateToProps, mapDispatchToProps)(App);
