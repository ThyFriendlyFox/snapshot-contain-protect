# Adapters

<!-- The pluggable seam: the piece users swap to connect this project to
     their environment. Rename the file to what the repo calls the seam
     — adapters, providers, backends, drivers, integrations — and update
     ROUTING.md. Delete the file only if the project genuinely has no
     pluggable seam.

     Same shape for every adapter: what it does, when to use it, the
     exact config block to enable it. -->

An adapter <one sentence: what an adapter translates or connects>.
{{PROJECT_NAME}} has <N> adapters. Select one with the `<field>` field.

## <adapter-id>

<What it does, in 1–3 sentences. When to use it. The default adapter
comes first and must be the safe one — it starts nothing destructive.>

```json
{ "<field>": { "id": "<adapter-id>" } }
```

## <adapter-id>

…

## Writing a new adapter

<!-- The contract: which interface to implement, where it registers,
     which gate proves it works. A new adapter PR includes: the
     implementation, its row in this file, its config fields in
     CONFIGURATION.md, and a gate in VERIFICATION.md. -->
