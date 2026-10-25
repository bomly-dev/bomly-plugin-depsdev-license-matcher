# bomly-plugin-depsdev-license-matcher

deps.dev license matcher for [Bomly](https://github.com/bomly-dev/bomly-cli).

It fills in missing package license data from the
[deps.dev](https://deps.dev) API for npm, Maven, Go, PyPI, NuGet, RubyGems,
and Cargo packages. Packages that already carry licenses are left untouched.

> **Already inside the Bomly CLI.** This matcher ships embedded in the `bomly`
> binary as the built-in `depsdev-license-matcher` — you do not need to
> install this plugin to use deps.dev license enrichment. This repository is
> the matcher's home as a standalone module: the Bomly CLI consumes the same
> code in-process, and the plugin binary serves it to hosts that run matchers
> as managed subprocesses.

## Identity

- Plugin id / descriptor name: `depsdev-license-matcher` (alias: `deps.dev`)
- Kind: matcher
- Module path: `github.com/bomly-dev/bomly-plugin-depsdev-license-matcher`

## Network behavior

This matcher performs network calls **only during enrichment** (`bomly scan
--enrich`), never during audit-only runs:

- `https://api.deps.dev/v3alpha/versionbatch` — batched license lookups, at
  most 100 packages per request.

Results are cached on disk (default `~/.bomly/cache/licenses/depsdev`,
24h TTL). Cache failures are non-fatal: the matcher logs a warning and
continues without caching.

## Access

`bomly-plugin.json` declares what the matcher touches, which Bomly shows when you install it and in `bomly plugin info`. It is a disclosure, not a sandbox.

| Access | Why |
| --- | --- |
| Reads and writes `home` | The response cache, `~/.bomly/cache/licenses/depsdev` by default |
| Network `api.deps.dev` | The deps.dev API the matcher queries |
| Environment `HOME`, `USERPROFILE` | `os.UserHomeDir` reads `HOME` (`USERPROFILE` on Windows) to locate the default cache |

Setting `cache_dir` or `api_base` moves the cache or the API to a place you chose; the declaration describes the defaults. The matcher needs no particular pipeline input beyond the packages it is asked to enrich, so it declares no requirements.

## Configuration

Embedded execution is configured through the Bomly CLI's own settings.
Managed execution reads a JSON block under
`plugins.matchers.depsdev-license-matcher`:

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `api_base` | string | `https://api.deps.dev/v3alpha` | deps.dev API base URL |
| `cache_dir` | string | `~/.bomly/cache/licenses/depsdev` | Result cache |
| `cache_ttl` | duration string | `24h` | Cache TTL |

## Package-updates delta protocol

The matcher advertises `package-updates-v1`. When the host sets
`AcceptPackageUpdates`, `Match` leaves the request registry untouched and
returns one delta per enriched package (PURL, `Matched`, and the license
list). This is safe because the only mutation the legacy in-place path
performs — filling `Licenses` on packages that have none — is exactly the
fill-when-empty rule `Package.MergeFrom` applies. The equivalence is pinned
by `TestMatchDeltaEquivalence`.

## Development

```sh
make test    # unit tests + SDK conformance suite
make build   # build bin/bomly-plugin-depsdev-license-matcher
```

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
