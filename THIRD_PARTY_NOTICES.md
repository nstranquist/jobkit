# Third-party notices

JobKit is MIT-licensed. Its compiled non-standard-library dependency set is
small and is checked by `go run ./tools/license-audit`.

| Module | Version | License | Upstream copyright |
|---|---|---|---|
| `go.yaml.in/yaml/v3` | `v3.0.5` | MIT and Apache-2.0 | Kirill Simonov and contributors |
| `golang.org/x/net` | `v0.58.0` | BSD-3-Clause | Copyright 2009 The Go Authors |
| `golang.org/x/sys` | `v0.47.0` | BSD-3-Clause | Copyright 2009 The Go Authors |

The complete upstream license texts remain in each module distribution. The
license audit pins their SHA-256 digests so upgrades require an explicit review.
