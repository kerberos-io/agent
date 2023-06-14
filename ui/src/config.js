const { hostname, host, protocol } = window.location;
const websocketprotocol = protocol === 'http:' ? 'ws:' : 'wss:';

// Uncomment this when using codespaces or other special DNS names (which you can't control)
// replace this with the DNS name of the kerberos agent server (the codespace url)
// const externalHost = 'xxx-8080.preview.app.github.dev';

const dev = {
  ENV: 'dev',
  // Comment the below lines, when using codespaces or other special DNS names (which you can't control)
  HOSTNAME: hostname,
  API_URL: `${protocol}//${hostname}:8080/api`,
  URL: `${protocol}//${hostname}:8080`,
  WS_URL: `${websocketprotocol}//${hostname}:8080/ws`,
  MODE: window['env']['mode'],
  // Uncomment, and comment the above lines, when using codespaces or other special DNS names (which you can't control)
  // HOSTNAME: externalHost,
  // API_URL: `${protocol}//${externalHost}/api`,
  // URL: `${protocol}//${externalHost}`,
  // WS_URL: `${websocketprotocol}//${externalHost}/ws`,
};

const prod = {
  ENV: process.env.REACT_APP_ENVIRONMENT,
  HOSTNAME: hostname,
  API_URL: `${protocol}//${host}/api`,
  URL: `${protocol}//${host}`,
  WS_URL: `${websocketprotocol}//${host}/ws`,
  MODE: window['env']['mode'],
};

const config = process.env.REACT_APP_ENVIRONMENT === 'production' ? prod : dev;

export default {
  // Add common config values here
  // MAX_ATTACHMENT_SIZE: 5000000,
  ...config,
};
