# Releases

Every push to `main` runs `.github/workflows/release.yml`. It reruns the validation from `validation-go.yml`, then `semantic-release` reads the commits since the last `v*` tag and decides the next version from their conventional commit types.

| Commit                                  | Release |
| --------------------------------------- | ------- |
| `fix` or `perf`                         | Patch   |
| `feat`                                  | Minor   |
| `BREAKING CHANGE:` footer or `!` suffix | Major   |
| Anything else                           | None    |

When a release is due, `semantic-release` pushes the `vX.Y.Z` tag and runs `goreleaser release --clean` through `@semantic-release/exec`. GoReleaser builds `revunk` for Linux and macOS on `amd64` and `arm64`, packages each build as a `.tar.gz`, adds a `.deb` for each Linux build, and publishes them with `checksums.txt` and a grouped changelog on the GitHub release.

## Configuration

| File                            | Role                                                    |
| ------------------------------- | ------------------------------------------------------- |
| `.github/workflows/release.yml` | Workflow, triggered by pushes to `main`, with no inputs |
| `.releaserc.json`               | `semantic-release` branches and plugins                 |
| `.goreleaser.yaml`              | Build targets, archives, `.deb` package, and changelog  |

The workflow needs no secrets beyond the built-in `GITHUB_TOKEN`, which it gives `contents: write` to push the tag and create the release. Tag rulesets must let GitHub Actions push `v*` tags. With no tag yet, the first release is `v1.0.0`.

Run `goreleaser release --snapshot --clean` to build every artifact locally into `dist/` without publishing.
