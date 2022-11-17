const { hostname, host, protocol } = window.location;

// Uncomment this when using codespaces or other special DNS names (which you can't control)
// replace this with the DNS name of the kerberos agent server (the codespace url)
// const externalHost = 'xxx-8080.preview.app.github.dev';

const dev = {
  ENV: 'dev',
  HOSTNAME: hostname,
  API_URL: `${protocol}//${hostname}:8080/api`,
  URL: `${protocol}//${hostname}:8080`,

  // Uncomment, and comment the above lines, when using codespaces or other special DNS names (which you can't control)
  // API_URL: `${protocol}//${externalHost}/api`,
  // URL: `${protocol}//${externalHost}`,
};

const prod = {
  ENV: process.env.REACT_APP_ENVIRONMENT,
  HOSTNAME: hostname,
  API_URL: `${protocol}//${host}/api`,
  URL: `${protocol}//${host}`,
};

const config = process.env.REACT_APP_ENVIRONMENT === 'production' ? prod : dev;

export default {
  // Add common config values here
  // MAX_ATTACHMENT_SIZE: 5000000,
  ...config,
};
