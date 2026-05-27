import React, { Suspense } from 'react';
import ReactDOM from 'react-dom';
import { Route, Switch } from 'react-router-dom';
import { createStore, applyMiddleware } from 'redux';
import { createBrowserHistory } from 'history';
import { routerMiddleware, ConnectedRouter } from 'connected-react-router';
import { Provider } from 'react-redux';
import reduxWebsocket from '@giantmachines/redux-websocket';
import { composeWithDevTools } from 'redux-devtools-extension';
import thunk from 'redux-thunk';
import { Redirect } from 'react-router';
import rootReducer from './reducers';
import App from './App';
import './index.scss';
import Login from './pages/Login/Login';
import Dashboard from './pages/Dashboard/Dashboard';
import Media from './pages/Media/Media';
import Settings from './pages/Settings/Settings';
import RequireAuth from './containers/RequireAuth';
import RequireGuest from './containers/RequireGuest';
import './i18n';

const history = createBrowserHistory();

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

const store = createStore(
  rootReducer(history),
  { ...getAuthState() },
  composeWithDevTools(
    applyMiddleware(thunk, reduxWebsocketMiddleware, routerMiddleware(history))
  )
);

const Loader = () => <div>loading...</div>;

ReactDOM.render(
  <Provider store={store}>
    <ConnectedRouter history={history}>
      <Switch>
        <Route path="/login" component={RequireGuest(Login)} />
        <Suspense fallback={<Loader />}>
          <App>
            <Route exact path="/" component={RequireAuth(Dashboard)} />
            <Route exact path="/dashboard" component={RequireAuth(Dashboard)} />
            <Route exact path="/" render={() => <Redirect to="/dashboard" />} />
            <Route exact path="/media" component={RequireAuth(Media)} />
            <Route exact path="/settings" component={RequireAuth(Settings)} />
          </App>
        </Suspense>
      </Switch>
    </ConnectedRouter>
  </Provider>,
  document.getElementById('root')
);
