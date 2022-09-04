import React from 'react';
import PropTypes from 'prop-types';
import { withTranslation } from 'react-i18next';
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
import { Link } from 'react-router-dom';
import { logout } from './actions';
import config from './config';
import { getDashboardInformation } from './actions/agent';
import LanguageSelect from './components/LanguageSelect/LanguageSelect';
import logo from './header-minimal-logo-36x36.svg';
import '@kerberos-io/ui/lib/index.css';
import './App.scss';

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
    const { t } = this.props;
    const { children, username, dashboard, dispatchLogout } = this.props;
    return (
      <div id="page-root">
        <Sidebar logo={logo} title="Kerberos Agent" version="v1-beta" mobile>
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
          </Navigation>
        </Sidebar>
        <Main>
          <Gradient />

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
    dispatch(getDashboardInformation(dashboard, success, error)),
});

App.propTypes = {
  t: PropTypes.func.isRequired,
  dispatchLogout: PropTypes.func.isRequired,
  // eslint-disable-next-line react/forbid-prop-types
  children: PropTypes.array.isRequired,
  username: PropTypes.string.isRequired,
  dashboard: PropTypes.objectOf(PropTypes.object).isRequired,
  dispatchGetDashboardInformation: PropTypes.func.isRequired,
};

export default withTranslation()(
  connect(mapStateToProps, mapDispatchToProps)(App)
);
