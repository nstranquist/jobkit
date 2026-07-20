# Third-party notices

JobKit is MIT-licensed. Its compiled non-standard-library dependency set is
small and is checked by `go run ./tools/license-audit`.

| Module | Version | License | Upstream copyright |
|---|---|---|---|
| `golang.org/x/net` | `v0.56.0` | BSD-3-Clause | Copyright 2009 The Go Authors |
| `gopkg.in/yaml.v3` | `v3.0.1` | MIT and Apache-2.0 | Kirill Simonov and contributors |

The complete upstream license texts remain in each module distribution. The
license audit pins their SHA-256 digests so upgrades require an explicit review.
