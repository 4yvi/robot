# Contributing

Thanks for helping! 

- Use Go 1.26+. Run `go vet ./...`, `go test ./...` and `golangci-lint run` before opening a PR.
- Commits follow [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, `chore:` ...). Release notes and versions are generated from them by release-please, so never tag manually.
- Put platform-specific code in files with build tags (`*_windows.go`, `*_darwin.go`).
- Each component lives in its own package behind an interface.
- Never commit secrets, models, `config.yaml` or personal data.
