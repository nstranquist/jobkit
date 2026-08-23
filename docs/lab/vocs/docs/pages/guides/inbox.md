---
title: Inbox
description: Pre-application queue, slate, form-fill, outreach drafts.
---

The inbox is the pre-application queue. IDs are stable. A repeat save of the same posting does not create a second row.

```sh
jobkit inbox list
jobkit inbox show <id>
jobkit inbox set <id> --status shortlisted
jobkit inbox note <id> "hiring manager is ex-Bayer"
jobkit inbox form <id> --format js --out auto
jobkit inbox outreach <id> --channel linkedin --out auto
```

`inbox form --format js` emits a DevTools snippet. The snippet fills standard Greenhouse, Lever, and label-matched fields. It never touches uploads, EEO fields, or submit.

Inbox status moves through `new`, `shortlisted`, `planned`, and `applied`. A repeated `apply-plan` on an active item reuses the tracker record. It does not move an applied, skipped, or archived item back to `planned`.
