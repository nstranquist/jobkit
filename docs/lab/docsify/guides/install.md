---
title: Install
description: Build JobKit and confirm the version.
---

Build the binary from the JobKit repository. Then put it on your `PATH`.

```sh
cd ~/tools/jobkit
make install
jobkit version
```

The default state directory is `~/.jobkit`. Set `JOBKIT_HOME` to use a different directory.

:::tip[Proof]
`jobkit version` prints `0.10.1`. `make demo` runs the full walkthrough in an isolated home and never reads your real state.
:::
