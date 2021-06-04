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

export const logout = () =>
  function dispatcher(dispatch) {
    dispatch({
      type: 'LOGOUT',
    });
    dispatch(push('/login'));
  };
