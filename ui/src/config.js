const { hostname, protocol } = window.location;

const dev = {
  ENV: 'dev',
  HOSTNAME: hostname,
  API_URL: `${protocol}//${hostname}:8080/`,
};

const prod = {
  ENV: process.env.REACT_APP_STAGE,
  HOSTNAME: hostname,
  API_URL: `/api`,
};

const config = process.env.REACT_APP_STAGE === 'production' ? prod : dev;

export default {
  // Add common config values here
  // MAX_ATTACHMENT_SIZE: 5000000,
  ...config,
};
