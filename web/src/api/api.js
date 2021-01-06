import axios from 'axios';
import config from '../config';
// import createAuthRefreshInterceptor from 'axios-auth-refresh';

const axiosBase = axios.create({
  baseURL: config.API_URL,
});

axiosBase.interceptors.request.use((request) => {
  request.headers.Authorization = `Bearer ${localStorage.getItem('token')}`;
  return request;
});

export default axiosBase;

/* const refreshAuthLogic = failedRequest => axiosBase.get('refresh_token')
    .then(tokenRefreshResponse => {
      localStorage.setItem('token', tokenRefreshResponse.data.token);
      localStorage.setItem('expire', tokenRefreshResponse.data.expire);
      failedRequest.response.config.headers['Authentication'] = \
       'Bearer ' + tokenRefreshResponse.data.token;
      return Promise.resolve();
});

createAuthRefreshInterceptor(a, refreshAuthLogic); */

export function getToken() {
  return localStorage.getItem('token');
}

export function getAPIObject(url) {
  return {
    url: config.API_URL + url,
    headers: {
      Authorization: `Bearer ${getToken()}`,
    },
  };
}

export function getAPI(baseURL) {
  const a = axios.create({
    baseURL,
  });
  axiosBase.interceptors.request.use((request) => {
    request.headers.Authorization = `Bearer ${getToken()}`;
    return request;
  });
  return a;
}
