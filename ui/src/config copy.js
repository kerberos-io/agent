const { hostname } = window.location;
const { protocol } = window.location;
const websocketprotocol = protocol === 'http:' ? 'ws:' : 'wss:';

const dev = {
  ENV: 'dev',
  HOSTNAME: hostname,
  API_URL: `${protocol}//${hostname}:8082/`,
  WS_URL: `${websocketprotocol}//${hostname}:8082/ws`,
};

const prod = {
  ENV: 'prod',
  HOSTNAME: hostname,
  API_URL:
    window.env.apiUrl !== ''
      ? `${protocol}//${window.env.apiUrl}/`
      : `${protocol}//api.${hostname}/`, // "https://staging-api.factory.kerberos.live", //
  WS_URL:
    window.env.apiUrl !== ''
      ? `${websocketprotocol}//${window.env.apiUrl}/ws`
      : `${websocketprotocol}//api.${hostname}/ws`,
};

const config = process.env.REACT_APP_STAGE === 'production' ? prod : dev;

export default {
  // Add common config values here
  // MAX_ATTACHMENT_SIZE: 5000000,
  ...config,
};
