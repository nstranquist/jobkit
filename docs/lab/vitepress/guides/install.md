---
title: Install
description: Build JobKit and confirm the version.
---

# Install <Badge type="info" text="local binary" />

Build the binary from the JobKit repository. Then put it on your `PATH`.

::: code-group

```sh [make]
cd ~/tools/jobkit
make install
jobkit version
```

```sh [go]
cd ~/tools/jobkit
go build -o "$HOME/.local/bin/jobkit" ./cmd/jobkit
jobkit version
```

:::

The default state directory is `~/.jobkit`. Set `JOBKIT_HOME` to use a different directory.

::: tip Proof
`jobkit version` prints `0.10.1`. `make demo` runs the full walkthrough in an isolated home and never reads your real state.
:::
