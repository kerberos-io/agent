import axios from 'axios';
import config from '../config';

const axiosBase = axios.create({
  baseURL: config.API_URL,
});

axiosBase.interceptors.request.use((request) => {
  request.headers.Authorization = `Bearer ${localStorage.getItem('token')}`;
  return request;
});

export default axiosBase;

export function getToken() {
  return localStorage.getItem('token');
}

// Browsers cannot attach an Authorization header to WebSocket connections or
// to media elements such as <video src>, so those URLs carry the token as the
// ?token= query parameter, which the agent's JWT middleware also accepts.
export function withToken(url) {
  const token = getToken();
  if (!token) {
    return url;
  }
  const separator = url.includes('?') ? '&' : '?';
  return `${url}${separator}token=${encodeURIComponent(token)}`;
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
