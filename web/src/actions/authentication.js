import { doLogin, doUpdateLicenseKey } from '../api/authentication';
import { push } from 'react-router-redux';

export const login = (username, password) => {
  return function (dispatch) {
    doLogin(username, password, (data) => {
      // mark interface as logged in.
      dispatch({
        type: 'LOGIN',
        username: data.username,
        role: data.role,
        token: data.token,
        expire: data.expire
      });
      dispatch(push('/'));
    }, (error, data) => {
      dispatch({
        type: 'LOGIN_FAILED',
        error: error.message
      });
    });
  }
}

export const logout = () => {
  return function (dispatch) {
    dispatch({
      type: 'LOGOUT'
    });
    dispatch(push('/login'));
  }
}
