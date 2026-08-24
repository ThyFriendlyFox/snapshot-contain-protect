# Use cases

Each case has the same shape. **Mark** is the checkpoint the agent takes.
**Act** is the work that follows. **Undo** is what the agent does when the
work goes wrong.

## Undo a file operation

**Mark.** The agent calls `POST /snapshot` before it deletes, moves or
rewrites files. The call returns in under 1 second.
**Act.** The agent runs the operation.
**Undo.** The agent calls `POST /restore` with the snapshot identifier and
`confirm: true`. The working set returns to its prior state.

## Decide whether to roll back

**Mark.** The agent snapshots before the action.
**Act.** The agent runs the action, then snapshots again.
**Undo.** The agent calls `GET /diff` between the two snapshots. It reads the
added, modified and deleted paths, then decides. A diff costs less than a
rollback.

## Checkpoint every step of a long task

**Mark.** The agent snapshots before each step with `auto: true`.
**Act.** The agent runs the step.
**Undo.** The agent lists the graph, finds the last good node, and restores
it. Retention keeps the last 50 auto checkpoints, so a long task does not
fill the disk.

## Keep a known-good state by hand

**Mark.** A person runs `snapctl snapshot proj-a "before the migration"`. A
labelled snapshot has `auto: false`.
**Act.** The agent works for hours.
**Undo.** Retention never removes the labelled snapshot, and never removes
the snapshots it descends from. The known-good state stays reachable.

## Branch instead of losing work

**Mark.** The agent restores an older node.
**Act.** The restore appends a new node under the restored one. The work that
followed the original node stays in the graph as a sibling branch.
**Undo.** The agent restores a node on either branch. Nothing was deleted.

## Recover from a wrong rollback

**Mark.** The service takes a safety snapshot before it restores.
**Act.** The agent restores the wrong node.
**Undo.** The agent restores the safety snapshot, labelled "before restore of
&lt;id&gt;", and is back where it started.

## Free disk space

**Mark.** The graph holds snapshots a person no longer needs.
**Act.** The person runs `snapctl prune <id>`, or `--cascade` for a subtree.
**Undo.** None. A prune is the one operation that removes history, which is
why it refuses a node with children until the caller asks for the subtree.

## Recover the graph after a restart

**Mark.** The agent works while the daemon runs.
**Act.** The daemon stops, by a crash or a restart.
**Undo.** The daemon reads `snapshot.db` at start. The graph, the labels and
the parent links are unchanged.
