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
} from '@kerberos-io/ui';
import { connect } from 'react-redux';
import './App.module.scss';
import logo from './header-minimal-logo-36x36.svg';
import '@kerberos-io/ui/lib/index.css';
import { logout } from './actions';
import config from './config';

// eslint-disable-next-line react/prefer-stateless-function
class App extends React.Component {
  render() {
    const { children, username, dispatchLogout } = this.props;
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
          <MainBody>{children}</MainBody>
        </Main>
      </div>
    );
  }
}

const mapStateToProps = (state) => ({
  username: state.auth.username,
});

const mapDispatchToProps = (dispatch) => ({
  dispatchLogout: () => dispatch(logout()),
});

App.propTypes = {
  dispatchLogout: PropTypes.func.isRequired,
  // eslint-disable-next-line react/forbid-prop-types
  children: PropTypes.array.isRequired,
  username: PropTypes.string.isRequired,
};

export default connect(mapStateToProps, mapDispatchToProps)(App);
