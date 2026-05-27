import React, { Suspense } from 'react';
import { createRoot } from 'react-dom/client';
import { Router, Switch, Route, Redirect } from 'react-router-dom';
import { createStore, applyMiddleware, compose } from 'redux';
import { Provider } from 'react-redux';
import reduxWebsocket from '@giantmachines/redux-websocket';
import { thunk } from 'redux-thunk';
import rootReducer from './reducers';
import App from './App';
import './index.scss';
import Login from './pages/Login/Login';
import Dashboard from './pages/Dashboard/Dashboard';
import Media from './pages/Media/Media';
import Settings from './pages/Settings/Settings';
import RequireAuth from './containers/RequireAuth';
import RequireGuest from './containers/RequireGuest';
import history from './history';
import './i18n';

// We get the token from the store to initialise the store.
// So we know if the user is still signed in.
function getAuthState() {
  try {
    const token = localStorage.getItem('token') || null;
    const expire = localStorage.getItem('expire') || null;
    const username = localStorage.getItem('username') || null;
    const role = localStorage.getItem('role') || null;
    // const installed = localStorage.getItem('installed') || null;
    const difference = new Date(expire) - new Date();
    const state = {
      authentication: {
        token,
        expire,
        username,
        role,
        loggedIn: difference >= 0,
        loginError: false,
        installed: true, //! !installed,
        error: '',
      },
    };
    return state;
  } catch (err) {
    return undefined;
  }
}

const reduxWebsocketMiddleware = reduxWebsocket({
  reconnectOnClose: true,
});

// eslint-disable-next-line no-underscore-dangle
const composeEnhancers = window.__REDUX_DEVTOOLS_EXTENSION_COMPOSE__ || compose;

const store = createStore(
  rootReducer,
  { ...getAuthState() },
  composeEnhancers(applyMiddleware(thunk, reduxWebsocketMiddleware))
);

const Loader = () => <div>loading...</div>;

const container = document.getElementById('root');
const root = createRoot(container);

root.render(
  <Provider store={store}>
    <Router history={history}>
      <Suspense fallback={<Loader />}>
        <Switch>
          <Route path="/login">
            <RequireGuest>
              <Login />
            </RequireGuest>
          </Route>
          <Route>
            <App>
              <Switch>
                <Route exact path="/">
                  <Redirect to="/dashboard" />
                </Route>
                <Route exact path="/dashboard">
                  <RequireAuth>
                    <Dashboard />
                  </RequireAuth>
                </Route>
                <Route exact path="/media">
                  <RequireAuth>
                    <Media />
                  </RequireAuth>
                </Route>
                <Route exact path="/settings">
                  <RequireAuth>
                    <Settings />
                  </RequireAuth>
                </Route>
              </Switch>
            </App>
          </Route>
        </Switch>
      </Suspense>
    </Router>
  </Provider>
);
