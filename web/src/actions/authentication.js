import { push } from 'react-router-redux';
import { doLogin, doUpdateLicenseKey } from '../api/authentication';

export const login = (username, password) => function (dispatch) {
  doLogin(username, password, (data) => {
    // mark interface as logged in.
    dispatch({
      type: 'LOGIN',
      username: data.username,
      role: data.role,
      token: data.token,
      expire: data.expire,
    });
    dispatch(push('/'));
  }, (error, data) => {
    dispatch({
      type: 'LOGIN_FAILED',
      error: error.message,
    });
  });
};

export const logout = () => function (dispatch) {
  dispatch({
    type: 'LOGOUT',
  });
  dispatch(push('/login'));
};
