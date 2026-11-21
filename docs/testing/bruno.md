# Bruno API checks

**Status:** implemented smoke suite

The repository includes a Git-tracked Bruno collection at
[`bruno/athena-api`](../../bruno/athena-api). It exercises the real Athena
HTTP boundary using the existing ConnectRPC JSON protocol:

- public system information and anonymous session checks;
- sign-in, cookie persistence and CSRF-token propagation;
- the authenticated Build permission gate for Record Types;
- audit-chain verification; and
- sign-out and session invalidation.

These checks complement Go and Vitest tests. Go tests are the fast, white-box
suite for business logic and database behavior; Bruno is the black-box contract
suite for the server process, HTTP headers, cookies, JSON field names and the
permission boundary. The suite does not create, update or delete business data.

## Run it

Start Athena in one terminal:

```sh
make run
```

Install the free Bruno CLI once (the Bruno desktop app and CLI are separate):

```sh
pnpm add --global @usebruno/cli
bru --version
```

Then run the collection from the repository root. The password is supplied at
runtime and is never stored in the collection:

```sh
ATHENA_BRUNO_PASSWORD='your administrator password' make bruno-test
```

Useful overrides are `ATHENA_BRUNO_USERNAME`, `ATHENA_BRUNO_BASE_URL` and
`BRUNO=bru`. The default target is `http://127.0.0.1:3000` and the default
username is `administrator`.

To run it directly in Bruno, open the `bruno/athena-api` folder, select the
`local` environment, and supply the `password` variable in the app. Do not
commit a populated environment file or a password.

The collection uses `.bru` files because they remain supported by Bruno 4.x
and produce reviewable Git diffs. Bruno's CLI can also emit JUnit output; the
Make target writes it to `/tmp/athena-bruno-junit.xml` so reports do not enter
the repository. The `local` environment sets `baseUrl` to
`http://127.0.0.1:3000`; CLI values supplied through `ATHENA_BRUNO_BASE_URL`
override it for another running Athena server.

On a disposable Workspace, set the Bruno environment variable `checkRateLimit`
to `true` to exercise the default sign-in bucket. Request 10 repeats an
incomplete Challenge request up to six times and checks HTTP 429 and the
`Retry-After` header. Set `checkIdleExpiry` to `true` for a separate expiry
run. Request 03 then jumps to request 09, which waits for the server's idle
deadline and checks HTTP 401 and the `idle` detail. Use a five-minute idle
policy on that Workspace for a shorter run.
