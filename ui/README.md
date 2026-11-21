# Kerberos Agent UI

The web interface of the Kerberos Agent: a React app built with
[Vite](https://vite.dev) and tested with [Vitest](https://vitest.dev).
Requires Node.js 20.19 or newer.

## Scripts

- `yarn start`: starts the dev server on
  [http://localhost:3000](http://localhost:3000) with hot reload.
- `yarn build`: builds for production and moves the output to
  `../machinery/www`, where the Agent serves it.
- `yarn preview`: serves the production build from `../machinery/www` on
  port 3000. Run `yarn build` first.
- `yarn test`: runs the test suite once. `yarn test:watch` keeps it running.
- `yarn lint`: lints `src/` with ESLint.
- `yarn format`: formats the code with Prettier.

## Configuration

In development the UI talks to the Agent API on port `8082` of the same host.
Override it with `VITE_AGENT_API_PORT`:

    VITE_AGENT_API_PORT=9090 yarn start

In production builds the UI uses the host and port it was served from.

Runtime settings that differ per deployment (such as demo mode) live in
`public/env.js`, which the Agent can replace without rebuilding the UI.

## Layout

- `index.html`: the HTML entry point; Vite injects the bundle here.
- `src/`: application code. `src/index.jsx` is the entry module.
- `public/`: copied unchanged to the build root (favicons, fonts, locales, `env.js`).
- Bundled JS, CSS and assets go to `static/` in the build, so they don't mix
  with the `assets/` folder from `public/`.
