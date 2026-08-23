# Style

<!-- The writing rules for every user-facing text: docs, UI copy, error
     messages, release notes. Based on Simplified Technical English.
     AGENTS.md house style governs code; this file governs words. -->

Snapshot uses one voice for every text.
The rules keep the text short, clear and easy to translate.

## Rules

1. Write one instruction in one sentence.
2. Keep an instruction under 20 words.
3. Keep a descriptive sentence under 25 words.
4. Use the active voice.
5. Use the simple present tense.
6. Use one word for one idea. Do not use synonyms.
7. Do not use contractions.
8. Do not use marketing words.
9. Start an instruction with the verb.
10. Use a list for a sequence of steps.
11. Use a table for a set of values.
12. Write numbers as digits.

## Command line and API text

Snapshot has no graphical interface. The user-facing text is the `snapctl`
output, the `-h` usage block, and the `error` field the daemon returns.

State the thing; never reassure about it. An error says what is wrong and
what the caller can do: "snapshot has children; set cascade=true to remove
the subtree". Do not add a hint line unless a human asks for one.

One exception carries a warning, because the filesystem cannot enforce it:
`snapctl restore` prints that processes and sockets do not roll back.

## Terms

One word for one idea, enforced. Grow this table as terms appear.

| Use | Do not use |
|---|---|
| snapshot | checkpoint, save point, restore point, backup |
| restore | roll back, revert, undo, rewind |
| prune | delete a snapshot, clean up, garbage collect |
| workset | working set, project, watched paths, scope |
| backend | adapter, driver, provider, plugin |
| handle | snapshot path, subvolume, shadow copy |
| graph | tree, history, timeline |
| auto snapshot | implicit checkpoint, automatic save |
| daemon | server, service, agent |
| the gate | CI, the checks, the build |

"Checkpoint" stays in one place: the container layer, where it names a CRIU
process checkpoint and not a filesystem snapshot.
