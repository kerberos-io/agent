import { push } from 'react-router-redux';
import { doLogin, doCheckIfInstalled } from '../api/authentication';

export const login = (username, password) =>
  function dispatcher(dispatch) {
    doLogin(
      username,
      password,
      (data) => {
        // mark interface as logged in.
        dispatch({
          type: 'LOGIN',
          username: data.username,
          role: data.role,
          token: data.token,
          expire: data.expire,
        });
        dispatch(push('/'));
      },
      (error) => {
        dispatch({
          type: 'LOGIN_FAILED',
          error: error.message,
        });
      }
    );
  };

export const checkIfInstalled = () =>
  function dispatcher(dispatch) {
    doCheckIfInstalled(
      (data) => {
        // Todo..
        dispatch({
          type: 'INSTALLED',
          installed: data,
        });
      },
      (error) => {
        // Todo..
        dispatch({
          type: 'INSTALLED_ERROR',
          error,
        });
      }
    );
  };

export const resetLogin = () => {
  return function (dispatch) {
    dispatch({
      type: 'RESET_LOGIN',
      error: null,
    });
  };
};

export const logout = () => {
  return function (dispatch) {
    dispatch({
      type: 'LOGOUT',
    });
    dispatch(push('/login'));
  };
};
