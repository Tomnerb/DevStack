# Contributing to Dockiva

Thank you for helping improve Dockiva. The project is under active
development, and native-runtime behavior can differ by operating system.

## Before opening a change

- Search existing issues and pull requests for related work.
- Open an issue before large architecture or runtime changes.
- Never include credentials, signing material, container data, or local guest
  images in a commit.
- Keep Dockiva Native and external Docker endpoint identities separate.
- Preserve unrelated work in the tree and keep commits focused.

Security vulnerabilities must follow [SECURITY.md](SECURITY.md), not the public
issue tracker.

## Development checks

Install the dependencies documented in [README.md](README.md), then run the
checks relevant to your change:

```bash
go test ./...
npm ci --prefix frontend
npm run build --prefix frontend
```

On Linux, install the GTK3 and WebKitGTK development packages documented in the
README and run `go test -tags gtk3 ./...`.

Run `gofmt` on changed Go files. Platform-specific engine changes should be
tested on the target operating system when possible; clearly document any
validation that could not be performed.

## Pull requests

Explain the user-visible behavior, implementation constraints, tests run, and
remaining platform limitations. Small, reviewable pull requests are preferred.

By submitting a contribution, you agree that it is licensed under the Apache
License 2.0 in [LICENSE](LICENSE).
