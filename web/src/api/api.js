import axios from 'axios';
//import createAuthRefreshInterceptor from 'axios-auth-refresh';
import config from '../config.js';

const a = axios.create({
  baseURL: config.API_URL
});

a.interceptors.request.use(request => {
    request.headers['Authorization'] = "Bearer " + localStorage.getItem('token');
    return request;
});

/*const refreshAuthLogic = failedRequest => a.get('refresh_token').then(tokenRefreshResponse => {
    localStorage.setItem('token', tokenRefreshResponse.data.token);
    localStorage.setItem('expire', tokenRefreshResponse.data.expire);
    failedRequest.response.config.headers['Authentication'] = 'Bearer ' + tokenRefreshResponse.data.token;
    return Promise.resolve();
});

createAuthRefreshInterceptor(a, refreshAuthLogic);*/

export default a;

export function getToken() {
  return localStorage.getItem('token')
}

export function getAPIObject(url) {
  return {
     url: config.API_URL + url,
     headers: {
      Authorization: "Bearer " + getToken(),
     },
  };
}

export function getAPI(baseURL) {
  const a = axios.create({
    baseURL,
  });
  a.interceptors.request.use(request => {
      request.headers['Authorization'] = "Bearer " + getToken();
      return request;
  });
  return a
}
