const assert = require('node:assert/strict');
const path = require('node:path');
const fs = require('node:fs');
const { createRequire } = require('node:module');
const { test } = require('node:test');
const vm = require('node:vm');

const requireFromCRA = createRequire(
  require.resolve('react-scripts/package.json')
);
const resolveUrlLoader = requireFromCRA('resolve-url-loader');
const { SourceMapGenerator } = createRequire(
  requireFromCRA.resolve('resolve-url-loader')
)('source-map');

for (const sourceMap of [false, true]) {
  test(`SCSS URL resolution preserves nested assets and query strings (sourceMap=${sourceMap})`, async () => {
    const root = path.resolve(__dirname, '../src');
    const content =
      '.fixture { background: url("../../header-minimal-logo-36x36.svg?theme=dark#icon"); }';
    const map = new SourceMapGenerator({ file: path.join(root, 'App.scss') });
    map.addMapping({
      generated: { line: 1, column: 0 },
      original: { line: 1, column: 0 },
      source: path.join(root, 'components/LanguageSelect/LanguageSelect.scss'),
    });
    map.setSourceContent(
      path.join(root, 'components/LanguageSelect/LanguageSelect.scss'),
      content
    );

    const result = await new Promise((resolve, reject) => {
      resolveUrlLoader.call(
        {
          resourcePath: path.join(root, 'App.scss'),
          context: root,
          rootContext: path.resolve(__dirname, '..'),
          fs,
          sourceMap,
          query: { sourceMap, root },
          getOptions() {
            return this.query;
          },
          cacheable() {},
          emitWarning: reject,
          async() {
            return (error, css, outputMap) => {
              if (error) reject(error);
              else resolve({ css, outputMap });
            };
          },
        },
        content,
        map.toJSON()
      );
    });

    assert.match(
      result.css,
      /url\("\.\/header-minimal-logo-36x36\.svg\?theme=dark#icon"\)/
    );
    assert.equal(Boolean(result.outputMap), sourceMap);
  });
}

for (const parent of ['css-minimizer-webpack-plugin', 'rollup-plugin-terser']) {
  test(`${parent} serializes worker options with functions and regular expressions`, () => {
    const requireFromParent = createRequire(requireFromCRA.resolve(parent));
    const serialize = requireFromParent('serialize-javascript');
    const version = requireFromParent(
      'serialize-javascript/package.json'
    ).version;
    assert.match(version, /^7\./);

    const options = {
      comments: /^keep/,
      minify: (input) => input.trim(),
      nested: { passes: 2 },
    };
    const restored = vm.runInNewContext(`(${serialize(options)})`);

    assert.equal(restored.comments.test('keep license'), true);
    assert.equal(restored.comments.test('discard comment'), false);
    assert.equal(restored.minify(' body '), 'body');
    assert.equal(restored.nested.passes, 2);
  });
}

test('Workbox Rollup minification passes serialized options through its real worker', async () => {
  const { rollup } = requireFromCRA('rollup');
  const { terser } = requireFromCRA('rollup-plugin-terser');
  const bundle = await rollup({
    input: 'compatibility-entry',
    plugins: [
      {
        name: 'compatibility-fixture',
        resolveId: (id) => (id === 'compatibility-entry' ? id : null),
        load: () =>
          '/*! keep license */ export const double = (value) => value * 2;',
      },
      terser({
        numWorkers: 1,
        mangle: false,
        compress: { passes: 2 },
        format: { comments: /keep/ },
      }),
    ],
  });

  try {
    const { output } = await bundle.generate({
      format: 'cjs',
      sourcemap: true,
    });
    const exports = {};
    vm.runInNewContext(output[0].code, { exports });

    assert.equal(exports.double(21), 42);
    assert.match(output[0].code, /2\*value/);
    assert.ok(output[0].map);
  } finally {
    await bundle.close();
  }
});

test('CRA SVG exports render correctly through webpack-dev-server and its upgraded middleware', async () => {
  process.env.NODE_ENV = 'production';
  const webpack = requireFromCRA('webpack');
  const DevServer = requireFromCRA('webpack-dev-server');
  const React = require('react');
  const { renderToStaticMarkup } = require('react-dom/server');
  const craConfig = requireFromCRA('./config/webpack.config.js')('production');
  const svgRule = craConfig.module.rules
    .find((rule) => rule.oneOf)
    .oneOf.find(
      (rule) => rule.test instanceof RegExp && rule.test.test('icon.svg')
    );
  assert.ok(svgRule);

  const logoPath = path.resolve(
    __dirname,
    '../src/header-minimal-logo-36x36.svg'
  );
  const entry = `import url, { ReactComponent as Logo } from ${JSON.stringify(
    logoPath
  )}; export { url, Logo };`;
  const compiler = webpack({
    mode: 'development',
    target: 'node',
    entry: `data:text/javascript,${encodeURIComponent(entry)}`,
    output: {
      path: path.resolve(__dirname, '.dependency-webpack-output'),
      filename: 'compatibility.js',
      publicPath: '/compiled/',
      library: { type: 'commonjs2' },
    },
    externals: { react: 'commonjs react' },
    module: { rules: [{ ...svgRule, issuer: undefined }] },
    infrastructureLogging: { level: 'error' },
    stats: 'errors-only',
  });
  const server = new DevServer(
    {
      host: '127.0.0.1',
      port: 0,
      static: false,
      hot: false,
      client: false,
      liveReload: false,
      webSocketServer: false,
      setupExitSignals: false,
      devMiddleware: { publicPath: '/compiled/', stats: 'errors-only' },
    },
    compiler
  );

  try {
    await server.start();
    const stats = await new Promise((resolve) =>
      server.middleware.waitUntilValid(resolve)
    );
    assert.equal(
      stats.hasErrors(),
      false,
      stats.toString({ all: false, errors: true })
    );

    const baseURL = `http://127.0.0.1:${
      server.server.address().port
    }/compiled/`;
    const response = await fetch(`${baseURL}compatibility.js`);
    assert.equal(response.status, 200);
    const module = { exports: {} };
    vm.runInNewContext(await response.text(), {
      module,
      exports: module.exports,
      require,
    });
    const { Logo, url } = module.exports;
    assert.equal(Logo.$$typeof, Symbol.for('react.forward_ref'));
    const markup = renderToStaticMarkup(
      React.createElement(Logo, {
        title: 'Camera icon',
        titleId: 'camera-title',
        ref: React.createRef(),
      })
    );
    assert.match(markup, /<svg/);
    assert.match(markup, /<title id="camera-title">Camera icon<\/title>/);
    assert.match(markup, /viewBox=/);
    assert.match(markup, /<path/);
    assert.match(url, /^\/compiled\/static\/media\/.*\.svg$/);
    assert.equal((await fetch(new URL(url, baseURL))).status, 200);
    assert.equal((await fetch(`${baseURL}missing.js`)).status, 404);
    await new Promise((resolve) => server.middleware.invalidate(resolve));
    assert.equal((await fetch(`${baseURL}compatibility.js`)).status, 200);
  } finally {
    await server.stop();
  }
});
