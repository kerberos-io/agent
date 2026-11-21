# Vendored packages

## `kerberos-io-ui-1.77.1.tgz`

`@kerberos-io/ui` 1.77.1 moves the library's build tools (`@svgr/webpack`,
`svg-react-loader`, `sass-loader`, Storybook, Rollup plugins) out of its
runtime `dependencies`. Those tools pulled vulnerable `svgo` 1 and
`decode-uri-component` 0.2 into the agent's lockfile.

1.77.1 is tagged on GitHub but was never published to npm (the publish
workflow's npm token is no longer valid), so the agent installs it from this
tarball instead of the registry.

- Source: [kerberos-io/ui](https://github.com/kerberos-io/ui) at commit
  `ae1faa7` (tag `v1.77.1`)
- Built with: `npm ci --legacy-peer-deps && npm run build && npm pack` (Node 20)
- SHA-256: `0b4bbbf3a939fa8b57e275252f1b97de3babbebf6abdaf25ae74e88db7b2c0b5`
- Compared with the published 1.77.0: same exports; the only code change is
  the bundled `uuid` (8 → 11). Story type declarations are no longer shipped.

The Dockerfile copies `ui/vendor` before `yarn --frozen-lockfile`, because
`package.json` references the tarball with a `file:` path.

### Switching back to the registry

Once a version of `@kerberos-io/ui` with this fix (1.77.1 or later) is on npm:

1. In `ui/package.json`, set `"@kerberos-io/ui": "^1.77.1"`.
2. Run `yarn install` in `ui/` to update `yarn.lock`.
3. Delete this folder and the `COPY ui/vendor ./vendor` line in the Dockerfile.

### Updating the tarball

Build and pack a newer version from a checkout of `kerberos-io/ui`, replace
the file here, update the `file:` path in `ui/package.json` if the name
changed, run `yarn install`, and update the commit and checksum above.
