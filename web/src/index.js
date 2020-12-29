import React from 'react';
import ReactDOM from 'react-dom';
import { Route, Switch } from 'react-router-dom';
import { createStore, applyMiddleware } from 'redux';
import { createBrowserHistory } from 'history';
import { routerMiddleware, ConnectedRouter } from 'connected-react-router'
import { Provider } from 'react-redux';
import { composeWithDevTools } from 'redux-devtools-extension';
import rootReducer from './reducers'
import thunk from 'redux-thunk';
import App from './App';
import './index.css';
import Login from './pages/Login/Login';
import Install from './pages/Install/Install';
import Dashboard from './pages/Dashboard/Dashboard';
import RequireInstall from './containers/RequireInstall';
import RequireAuth from './containers/RequireAuth';
import RequireGuest from './containers/RequireGuest';
import './index.css';
export const history = createBrowserHistory();

// We get the token from the store to initialise the store.
// So we know if the user is still signed in.
function getAuthState() {
  try {
    const token = localStorage.getItem('token') || undefined;
    const expire = localStorage.getItem('expire') || undefined;
    const username = localStorage.getItem('username') || undefined;
    const role = localStorage.getItem('role') || undefined;
    const installed = localStorage.getItem('installed') || undefined;
    const difference = new Date(expire)-new Date();
    const state = {
      auth: {
        token,
        expire,
        username,
        role,
        loggedIn: difference >= 0,
        loginError: false,
        installed: !!installed,
        error: ""
      }
    };
    return state;
  } catch (err) { return undefined; }
}

const store = createStore(
  rootReducer(history),
  { ...getAuthState() },
  composeWithDevTools(
    applyMiddleware(
      thunk,
      routerMiddleware(history)
    )
  )
);

ReactDOM.render(
  <Provider store={store}>
    <ConnectedRouter history={history}>
      <Switch>
        <Route path="/install" component={RequireInstall(Install)} />
        <Route path="/login" component={RequireGuest(Login)} />
        <App>
          <Route exact path="/" component={RequireAuth(Dashboard)} />
        </App>
      </Switch>
    </ConnectedRouter>
  </Provider>,
  document.getElementById('root'));
