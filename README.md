# Secretli has moved

This repository is archived. Secretli now lives in the **[secretli](https://github.com/secretli)** organization, with one repository per part:

| Part | Repository |
|---|---|
| The web app at [secretli.app](https://secretli.app) | [secretli/web](https://github.com/secretli/web) |
| The server (API) | [secretli/server](https://github.com/secretli/server) |
| The `secretli` command | [secretli/cli](https://github.com/secretli/cli) |
| The encryption format: specification, Go and TypeScript libraries | [secretli/format](https://github.com/secretli/format) |
| Tests of the whole setup, and the smoke test of production | [secretli/e2e](https://github.com/secretli/e2e) |

Issues and changes go to the repository of the part they concern.

## The command line

Install the current `secretli` from [secretli/cli's releases](https://github.com/secretli/cli/releases), or with a Go toolchain:

```bash
go install github.com/secretli/cli/cmd/secretli@latest
```

The `cli-v0.1.0` release here and `go install github.com/pscheid92/secretli/cmd/secretli` keep working, but get no updates. Links work across both.

## History

Everything before the split, up to October 2026, stays in this repository's history.
