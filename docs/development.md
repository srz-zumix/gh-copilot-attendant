# Development

See the [project overview](../README.md) for installation and usage guides.

## Local checks

```sh
go test ./...
go run . --help
```

## Dashboard matcher

The canvas ships a standalone RE2JS matcher with its MIT license; end users do not need
npm or a runtime package installation. To update the pinned matcher bundle:

```sh
npm ci
npm run vendor:canvas
```
