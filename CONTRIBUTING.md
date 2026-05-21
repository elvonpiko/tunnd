# Contributing to Tunnd

Thanks for your interest in Tunnd. This document captures the workflow and conventions used in the project. Skim it once before opening your first PR; refer back as needed.

## Reporting bugs

1. Search [existing issues](https://github.com/elvonpiko/tunnd/issues) first — your bug may already be tracked.
2. If not, open a new issue using the **Bug Report** template. Include:
   - Tunnd version (`tunnd version` and `tunnd-server version`)
   - OS / distribution and architecture
   - Steps to reproduce, expected behavior, and actual behavior
   - Relevant logs (`journalctl -u tunnd` or `docker logs tunnd-server`)

## Suggesting features

Open an issue with the **Feature Request** template. Describe the use case first, then the proposed solution. Tunnd aims to stay small — features that would significantly expand scope (built-in metrics backends, SaaS-style multi-tenant billing, etc.) probably belong in a downstream tool.

## Security issues

Do **not** open a public issue. Use [GitHub Security Advisories](https://github.com/elvonpiko/tunnd/security/advisories/new) instead. See [SECURITY.md](SECURITY.md) for details.

## Submitting changes

### Workflow

1. Open or comment on an issue describing the problem you're solving — this avoids duplicate work and unsurprises during review.
2. Fork the repo and create a topic branch from `main`:
   ```bash
   git checkout -b fix/short-description
   ```
3. Make your change. Keep PRs focused — one logical change per PR.
4. Run the full local check before pushing:
   ```bash
   make fmt
   make vet
   make lint        # if you have golangci-lint installed
   make test
   ```
5. Push and open a PR against `main`. Fill in the PR template.

### Commit messages

Tunnd uses [Conventional Commits](https://www.conventionalcommits.org/). Common prefixes:

- `feat:` new feature visible to users
- `fix:` bug fix
- `perf:` performance improvement
- `docs:` documentation only
- `refactor:` neither fixes a bug nor adds a feature
- `test:` adds or fixes tests
- `ci:` CI configuration
- `chore:` build, dependency, or housekeeping change

Examples:

```
feat(tunnel): add max_tunnels_per_token enforcement
fix(client): handle MsgData arriving before local conn registered
perf(proto): tagged binary frames for stream payloads
```

The release changelog is generated automatically from these prefixes via GoReleaser.

### Code style

- Run `make fmt` (gofmt + goimports) before committing.
- `make lint` should pass without new warnings.
- Public APIs and exported types must have doc comments. The first sentence should start with the identifier name.
- Tests live next to the code they test (`foo.go` ↔ `foo_test.go`). Prefer table-driven tests with descriptive names.

### Testing

- Unit tests must pass on every PR (`make test`).
- For changes to the wire protocol or session lifecycle, add coverage in `internal/tunnel/registry_test.go` or `pkg/proto/`.
- For server-only changes, the docker image build is exercised by the `docker-build` workflow on push.

## Documentation

User-facing docs live under `docs/src/content/docs/`. They're built with [Astro Starlight](https://starlight.astro.build/). To preview locally:

```bash
cd docs
npm install
npm run dev
```

The deployed site at https://elvonpiko.github.io/tunnd/ uses the `base: /tunnd` path. Internal links should be written as `/some-page/` (without the `/tunnd` prefix) — Astro adds it automatically at build time.

## Development setup

```bash
git clone https://github.com/elvonpiko/tunnd
cd tunnd
go mod tidy
make build
```

Local dev server (no TLS):

```bash
TUNND_DOMAIN=localhost TUNND_HTTP_PORT=8081 TUNND_ADMIN_PORT=9091 \
  ./bin/tunnd-server

./bin/tunnd-server token create dev
./bin/tunnd setup        # wss://localhost:8081
./bin/tunnd http 3000
```

## Code of Conduct

Participation is governed by [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). Be respectful. Disagreements about technical direction are normal and welcome; personal attacks are not.

## License

By submitting a contribution, you agree that your work will be licensed under the [MIT License](LICENSE) used by this project.
