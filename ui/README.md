# Getting Started with Create React App

This project was bootstrapped with [Create React App](https://github.com/facebook/create-react-app).

## Available Scripts

In the project directory, you can run:

### `yarn start`

Runs the app in the development mode.\
Open [http://localhost:3000](http://localhost:3000) to view it in the browser.

The page will reload if you make edits.\
You will also see any lint errors in the console.

### `yarn test`

Launches the test runner in the interactive watch mode.\
See the section about [running tests](https://facebook.github.io/create-react-app/docs/running-tests) for more information.

### `yarn build`

Builds the app for production to the `build` folder.\
It correctly bundles React in production mode and optimizes the build for the best performance.

The build script then replaces `../machinery/www` with the generated assets.
Run it in an isolated checkout or container when validating dependencies.

The build is minified and the filenames include the hashes.\
Your app is ready to be deployed!

See the section about [deployment](https://facebook.github.io/create-react-app/docs/deployment) for more information.

## Dependency maintenance

Use the Node 22 image pinned in the root Dockerfile and Yarn Classic:

```sh
yarn install --frozen-lockfile
CI=true yarn test --watchAll=false --runInBand
yarn test:dependencies
yarn build
```

Keep the targeted `underscore` resolution: `jsonpath` pins a vulnerable version.
That resolution stays within the same major version. Axios remains on
its maintained 0.x line; `src/api/api.test.js` checks the HTTP behavior used by
the app without mocking Axios. Localization uses `i18next-xhr-backend`, not the
removed, unused `i18next-http-backend` dependency.

Additional targeted build-time upgrades are validated against the pinned
Node 22 / webpack 5 toolchain:

- [`resolve-url-loader` 5](https://github.com/bholloway/resolve-url-loader/releases/tag/5.0.0)
  upgrades PostCSS to 8, drops Node <12 / webpack <4, and removes the deprecated
  `rework` engine. CRA uses the supported PostCSS engine and unchanged `root` /
  `sourceMap` options. Dependency tests check relative SCSS asset URLs, query
  strings and fragments with source maps enabled and disabled.
- [`serialize-javascript` 7](https://github.com/yahoo/serialize-javascript/releases/tag/v7.0.0)
  requires Node >=20, matching this UI's declared minimum. Its CommonJS serializer
  API remains compatible with CSS minimizer and Rollup worker options.
  Versions 5/6 changed sparse-array and URL serialization, neither of which these
  callers use. Tests cover both consumers' function / regular-expression
  serialization and the real Workbox Rollup minification worker; the production
  build exercises CSS minimization. The targeted resolutions use patched 7.1.4.
- [`@svgr/webpack` 6](https://react-svgr.com/docs/migrate/) supports webpack 5 and
  the existing `file-loader` combination. CRA disables SVGO and does not use a
  custom template or call `@svgr/core`, so the documented breaking changes do
  not affect this pipeline. Tests use CRA's actual SVG rule to check asset URLs,
  React 17 rendering, viewBox preservation, titles and forwarded refs.
- [`webpack-dev-middleware` 7](https://github.com/webpack/webpack-dev-middleware/releases/tag/v7.0.0)
  raises the Node minimum and upgrades its internal memory filesystem; version 6
  raised the webpack minimum to 5. Both requirements match this toolchain, and
  webpack-dev-server remains on CRA's 4.x line. Tests exercise real server
  compilation, in-memory asset serving, missing assets and invalidation with
  patched middleware 7.4.6. No build artifacts are written by this test.

Refreshing the lockfile does not resolve every legacy-toolchain advisory.
Create React App 5, `@kerberos-io/ui`'s SVG tooling, and their transitive
dependencies still require upstream upgrades or explicit, tested migrations.
Do not force incompatible transitive major versions just to clear a scan.

The obsolete `svg-react-loader` dependency declared by `@kerberos-io/ui` is
replaced with the maintained `@svgr/webpack` 8.1.0 through a Yarn package alias.
This removes its `css -> source-map-resolve -> decode-uri-component` chain;
it does not force an incompatible ESM version of the decoder into a CommonJS
caller. The shared UI ships precompiled SVG components and does not invoke that
loader. CRA's active SVG rule remains on the separately tested SVGR 6 path.
SVGR 8.1.0 is also pinned explicitly as a development dependency, so dependency
scanners inventory and check its actual package identity even if their Yarn
alias parser reports an additional `svg-react-loader@8.1.0` entry. Tests verify
that both names resolve to identical package metadata; the alias is not used
to conceal an unscanned replacement.

This is a scoped consumer migration, **not** a general drop-in implementation of
the old loader's `raw`, `filters`, `propsMap` or other query options. Dependency
tests guard against shared UI code starting to call the legacy API, resolve the
alias from the shared package, and compile/render its maintained replacement
with React 17, titles, forwarded refs, viewBox and caller props. Rendering is
compared to CRA's actual SVG pipeline. Recheck these assumptions on shared UI
upgrades; use SVGR's supported options for any new loader calls.

### `yarn eject`

**Note: this is a one-way operation. Once you `eject`, you can’t go back!**

If you aren’t satisfied with the build tool and configuration choices, you can `eject` at any time. This command will remove the single build dependency from your project.

Instead, it will copy all the configuration files and the transitive dependencies (webpack, Babel, ESLint, etc) right into your project so you have full control over them. All of the commands except `eject` will still work, but they will point to the copied scripts so you can tweak them. At this point you’re on your own.

You don’t have to ever use `eject`. The curated feature set is suitable for small and middle deployments, and you shouldn’t feel obligated to use this feature. However we understand that this tool wouldn’t be useful if you couldn’t customize it when you are ready for it.

## Learn More

You can learn more in the [Create React App documentation](https://facebook.github.io/create-react-app/docs/getting-started).

To learn React, check out the [React documentation](https://reactjs.org/).

### Code Splitting

This section has moved here: [https://facebook.github.io/create-react-app/docs/code-splitting](https://facebook.github.io/create-react-app/docs/code-splitting)

### Analyzing the Bundle Size

This section has moved here: [https://facebook.github.io/create-react-app/docs/analyzing-the-bundle-size](https://facebook.github.io/create-react-app/docs/analyzing-the-bundle-size)

### Making a Progressive Web App

This section has moved here: [https://facebook.github.io/create-react-app/docs/making-a-progressive-web-app](https://facebook.github.io/create-react-app/docs/making-a-progressive-web-app)

### Advanced Configuration

This section has moved here: [https://facebook.github.io/create-react-app/docs/advanced-configuration](https://facebook.github.io/create-react-app/docs/advanced-configuration)

### Deployment

This section has moved here: [https://facebook.github.io/create-react-app/docs/deployment](https://facebook.github.io/create-react-app/docs/deployment)

### `yarn build` fails to minify

This section has moved here: [https://facebook.github.io/create-react-app/docs/troubleshooting#npm-run-build-fails-to-minify](https://facebook.github.io/create-react-app/docs/troubleshooting#npm-run-build-fails-to-minify)
