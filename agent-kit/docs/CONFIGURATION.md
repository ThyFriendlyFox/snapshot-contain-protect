# Configuration

<!-- Every knob, in one table, with defaults. If configuration lives in
     more than one place, this file lists all of them and which wins. -->

{{PROJECT_NAME}} reads <the config file/mechanism>. <How the user opens or edits it.>

| Platform | Path |
|---|---|
| macOS | `…` |
| Windows | `…` |
| Linux | `…` |

<Behavior rules: what happens at first start, with an incomplete file,
with a damaged file. A damaged config must never cause a crash —
defaults plus a log line.>

## Fields

| Field | Type | Default | Use |
|---|---|---|---|
| `…` | … | `…` | <one line> |

<!-- Rules for this table:
     · Every shipped option appears here — an undocumented option is a
       defect (gate it in VERIFICATION.md if the stack allows).
     · New options land in this table in the same PR that adds them.
     · Defaults shown here are the real defaults in code, not intended
       ones. -->
