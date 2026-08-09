# Contributing

Bug reports, documentation fixes, and small code changes are welcome.

## Before you start

- Search the existing issues before opening a new one.
- Open an issue before starting a large change. This gives us a chance to agree on the approach before you spend time on it.
- Never include API keys, personal workout data, or captured authenticated traffic.
- Use [GitHub's private vulnerability reporting](https://github.com/samuelfarkas/lyftacli/security/advisories/new) for security issues.

## Development

Lyfta CLI requires Go 1.25 or later.

```bash
git clone https://github.com/samuelfarkas/lyftacli.git
cd lyftacli
make verify
make build
./bin/lyfta capabilities --pretty
```

Tests use injected HTTP transports and do not require a Lyfta API key or network port.

## Pull requests

1. Create a branch from `master`.
2. Keep the change focused. Add tests for changed behavior or bug fixes.
3. Run `make verify`.
4. In the pull request, explain what changes for users and how you tested it.

All changes go through pull requests. Before a pull request can merge, the `verify` check must pass and every review conversation must be resolved.

By contributing, you agree that your contribution is licensed under the [MIT License](LICENSE).
