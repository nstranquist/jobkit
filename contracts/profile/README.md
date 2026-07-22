# JobKit profile compatibility contract

Contract version: `v1.0.0`

JobKit owns the serialized meaning of its core profile fields. Consumers may
read the YAML contract without importing JobKit's internal Go packages. The
canonical synthetic fixture is `v1.example.yaml`; it contains every JobKit v1
field and no personal data.

The supported integration direction is JobKit profile to consumer. A consumer
such as Resume Suite may add optional fields in its own copy, but those fields
are outside this contract. Consumers must not write an extended profile back to
JobKit unless they can prove that the extension fields are preserved or have
been intentionally removed.

Compatibility policy follows SemVer:

- patch: documentation or validation corrections that do not change fields;
- minor: new optional fields or relaxed validation;
- major: renamed or removed fields, changed field meaning, new required fields,
  or stricter validation that rejects a previously valid v1 profile.

The stable cross-product runtime boundary is the `jobkit --json` CLI envelope.
This YAML contract exists only for the explicit profile-import workflow; it is
not a shared source package and does not collapse repository ownership.

Verification:

```sh
go test ./internal/profile -run ProfileContract
JOBKIT_PROFILE="$PWD/contracts/profile/v1.example.yaml" jobkit profile validate
RESUME_SUITE_PROFILE="$PWD/contracts/profile/v1.example.yaml" resume-suite profile validate
```
