# FRNKSTN

Frnkstn is a chat app over ssh built with CharmCLI - wish, with gRPC for API on the backend and CharmCLI - bubble tea on the front-end.

## Shared API models

The schemas and generated Go/gRPC packages live in the separate
[`frnkstn-proto`](https://github.com/tobib-dev/frnkstn-proto) repository.
Both `api/client` and `api/server` import its `sessions/v1` and `users/v1` packages.

This app consumes the published model module at the commit pinned in `go.mod`.
Schemas and generated code are maintained in `frnkstn-proto`; this repository
does not need Buf, protobuf generators, or a local model checkout to build.

### Private repository access

Local development and CI need Git credentials with read access to
`tobib-dev/frnkstn-proto`. For GitHub CLI users, authenticate with `gh auth login`
and configure Git with `gh auth setup-git`. Include the model repository in
`GOPRIVATE` (preserve any other private module patterns you already use):

```sh
export GOPRIVATE=github.com/tobib-dev/frnkstn-proto
go mod download
go build ./...
```

### Updating the models

Edit, generate, validate, and push the models in `frnkstn-proto`. Then, in this
repository, select the published commit or tag:

```sh
go get github.com/tobib-dev/frnkstn-proto@<commit-or-tag>
go mod tidy
go test ./...
```

Commit the updated `go.mod` and `go.sum`. Both client and server use the same
pinned model version and remain separate executables in this app.

### Sign-in and account lookup

The SSH server uses the GitHub access token to fetch `/user`, then sends the
numeric GitHub ID to the API's `GetUserByGHID`. Only `NotFound` opens the username
prompt. `CreateUser` generates a local user ID, and `CreateSession` receives
that ID after the account exists. User lookup and creation are internal RPCs
that trust the GitHub identity supplied by the SSH server.

Apply `api/db/migrations/001_users_by_github_id.cql` to the existing ScyllaDB
keyspace before running this flow. Account creation uses `IF NOT EXISTS` on
GitHub ID so retries keep the same local account. Existing accounts need a
GitHub-ID mapping before they can be recognized by this lookup.

The protobuf additions for this flow are currently in the sibling
`frnkstn-proto` checkout. To develop against both modules, use a local workspace:

```sh
go work init . ../frnkstn-proto
go test ./...
```

If `go.work` already exists, use `go work use . ../frnkstn-proto` instead.
After publishing the protobuf changes, update the module version in `go.mod`
and verify with `GOWORK=off go test ./...` before shipping. The local `go.work`
and `go.work.sum` are ignored by Git.
