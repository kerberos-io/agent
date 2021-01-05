const { hostname } = window.location;
const { protocol } = window.location;

const dev = {
  ENV: 'dev',
  HOSTNAME: hostname,
  API_URL: `${protocol}//${hostname}:8080/`,
};

const prod = {
  ENV: 'prod',
  HOSTNAME: hostname,
  API_URL: `${protocol}//${hostname}:8080/`,
  // API_URL: window["env"]["apiUrl"] !== "" ? `${protocol}//${window["env"]["apiUrl"]}/` : `${protocol}//api.${hostname}/`,
};

const config = process.env.REACT_APP_STAGE === 'production'
  ? prod
  : dev;

export default {
  // Add common config values here
  // MAX_ATTACHMENT_SIZE: 5000000,
  ...config,
};
