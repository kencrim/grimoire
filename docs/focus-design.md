# Design: `ws focus` / `ws unfocus`

## Problem

The primary repo checkout (`RepoDir`) holds all the git-ignored environment
configuration and build artifacts (`.env`, `node_modules`, local config, build
caches) needed to actually *run* the project. Worktrees created by `ws add` are
clean checkouts that lack these artifacts, so `yarn dev` works in the main repo
but blows up in a worktree.

```
./main_repo        -- on `main`         -- has env config; `yarn dev` works
./worktree_folder  -- on `foo_feature`  -- missing git-ignored artifacts; `yarn dev` fails
```

We want to temporarily run a workstream's branch from inside the main repo,
because git won't let the same branch be checked out in two worktrees at once.

```
ws focus            # fuzzy-pick a workstream; borrow its branch into main_repo
ws focus foo        # non-interactive form
ws unfocus          # restore everything to its canonical branch
```

After `ws focus foo`:

```
./main_repo        -- on `foo_feature`  -- env config still present; can run foo
./worktree_folder  -- detached at foo_feature's tip  (parked)
```

## Key insight: this is the env-config-stays-put problem, not a content-copy problem

`git switch` between branches **never touches untracked / git-ignored files** —
they belong to the working directory, not to any branch. So switching
`main_repo` from `main` to `foo_feature` swaps the *tracked* content while
leaving `.env`, `node_modules`, and friends exactly where they are. That is
precisely the behavior we want, and it's why a branch swap (rather than copying
artifacts into the worktree) is the right tool.

Mapping to existing state — no new repo concept is needed:

| Diagram          | `ws` model                  |
| ---------------- | --------------------------- |
| `main_repo`      | `node.RepoDir`              |
| `worktree_folder`| `node.WorkDir`              |
| `foo_feature`    | `node.Branch` (canonical)   |

`ws focus foo` borrows `node.Branch` into `node.RepoDir`; the lender is
`node.WorkDir`.

## How the swap is performed

### Decision: park the lender on a *detached HEAD*, not a third branch, not `reset --hard`

The user asked which mechanism to use. The answer is **detached HEAD**, which is
a deliberate middle path between the two extremes considered:

- **Not `git reset --hard`.** Reset rewrites the working tree and *destroys*
  uncommitted work. Focus is a routine, reversible convenience — it must never
  be able to eat changes. Rejected.
- **Not a permanent third branch** (e.g. `ws-park/foo`). It works, but it
  pollutes the branch namespace, shows up in `git branch`, can drift, and leaves
  litter if a focus session is interrupted. Rejected as the default.
- **Detached HEAD (chosen).** Detaching the lender frees the branch *ref*
  without changing a single file in the lender's working tree (HEAD detaches at
  the *same commit* the branch points to). The branch is then free to be checked
  out in `main_repo`. Zero working-tree churn, zero namespace pollution, fully
  reversible.

### Why detach the lender instead of literally swapping it onto `main`

The sketch suggested `worktree_folder` could "sit on `main`". We deliberately
*don't* do that. Moving the lender onto `main` would rewrite its working tree to
`main`'s content for no benefit — the lender can't run anyway (no env config)
and the user isn't working in it while focused. Detaching at `foo_feature`'s tip
leaves the lender's files **untouched**, so it looks and behaves exactly as
before, minus the branch label. Less churn, fewer surprises.

### Focus sequence (`ws focus foo`)

Let `R = node.RepoDir` (main_repo), `W = node.WorkDir` (lender worktree),
`B = node.Branch` (foo_feature), and `P` = the branch `R` is currently on.

1. **Preconditions.**
   - Resolve `node` for `foo`; require `Type == local`.
   - Require no existing focus on `R` (or auto-unfocus it first — see below).
   - If `R` has uncommitted changes: abort with a clear message, unless
     `--autostash` is passed (then `git -C R stash push -u` and remember the
     stash ref). The lender `W` is *not* required to be clean — detaching at the
     same commit preserves its uncommitted changes in place.
2. **Free the branch.** In the lender: `git -C W switch --detach`
   (HEAD detaches at `B`'s tip; `W`'s files unchanged; `B` ref now free).
3. **Borrow the branch.** In the main repo: `git -C R switch B`
   (`P` ref now free; `B` checked out in `R`; git-ignored env files untouched).
4. **Persist the focus session** (see state below) recording `R`, `foo`, `B`,
   `P`, `W`, and any stash ref.
5. Print the result and the path to `cd` into:
   `Focused foo into <R> (was on P). Lender <W> parked (detached).`

If any git step fails, roll back the steps already taken so we never leave a
half-swapped repo.

### Unfocus sequence (`ws unfocus`)

1. Load the focus session for `R` (or, with no args, the single active session;
   error if none).
2. **Validate** that `R` is still on `B` and `W` is still detached at `B`'s tip.
   If reality has drifted (user hand-edited branches), warn and require
   `--force` rather than silently clobbering.
3. **Return the branch.** `git -C R switch P` (restore main_repo; `B` ref free).
4. **Re-attach the lender.** `git -C W switch B` (re-attaches at the same commit;
   any uncommitted changes parked in `W` come back with it).
5. If a stash was taken in step 1 of focus: `git -C R stash pop <ref>`.
6. Delete the focus session from state.

Because every move is a `git switch` between commits that are already present,
unfocus is always a fast-forward-free, no-merge operation.

## Tracking the canonical branch

There are two "current branch" facts to track, and they are tracked differently
on purpose:

- **The workstream's canonical branch is `node.Branch` — and focus never mutates
  it.** While focused, `W` is in detached HEAD, but `node.Branch` still reads
  `foo_feature`. That field *is* the source of truth we restore the lender to on
  unfocus. No new per-node field is required.
- **The main repo's pre-focus branch (`P`) is ephemeral.** We don't treat the
  main repo as having a permanent "canonical" branch; we only need to remember
  what it was on *at the moment of focus* so we can put it back. That snapshot
  lives in the focus session, not in any long-lived repo record.

### New state file: `~/.config/ws/focus.json`

Consistent with the existing one-file-per-concern pattern (`state.json`,
`repos.json`, `remotes.json`). A focus is scoped to a repo dir — a given
`RepoDir` can host at most one focus at a time — so the file is a map keyed by
absolute repo path:

```go
// libs/core/focus.go
type Focus struct {
    RepoDir        string    `json:"repo_dir"`         // main_repo, receiving the branch
    WorkstreamID   string    `json:"workstream_id"`    // node that lent its branch
    BorrowedBranch string    `json:"borrowed_branch"`  // == node.Branch (B)
    RepoPrevBranch string    `json:"repo_prev_branch"` // what R was on before focus (P)
    WorktreePath   string    `json:"worktree_path"`    // lender W, now detached
    StashRef       string    `json:"stash_ref,omitempty"`
    CreatedAt      time.Time `json:"created_at"`
}

type FocusRegistry struct {
    Active map[string]*Focus `json:"active"` // keyed by RepoDir
    path   string
}
```

This mirrors `RepoRegistry` (`Load`/`Save`/`Add`/`Get`/`Remove`), so the
implementation reuses a known shape in the codebase.

## CLI surface

```
ws focus [name]      Borrow a workstream's branch into its main repo (RepoDir).
                     No name -> fzf picker over local workstreams (reuses the
                     root.go tree-line + extractIDFromLine machinery).
    --autostash      Stash uncommitted changes in the main repo before switching,
                     and pop them on unfocus.

ws unfocus [name]    Restore the main repo and lender to their canonical branches.
                     No name -> the single active focus (errors if 0 or >1).
    --force          Proceed even if the live branch state has drifted from what
                     focus recorded.

ws status            Extended to show a "focused: foo -> /path/to/main_repo"
                     line when a focus session is active, so the swap is never
                     invisible.
```

### Interaction with the rest of `ws`

- **`ws list` / picker:** a focused workstream is annotated (e.g. `★ focused`)
  so it's obvious its lender worktree is parked.
- **Re-focusing:** `ws focus bar` while `foo` is focused *on the same repo*
  auto-unfocuses `foo` first, then focuses `bar` (one focus per repo invariant).
  Focusing into a *different* repo is independent and allowed concurrently.
- **`ws kill foo` while focused:** unfocus first (restore `R` to `P`), then tear
  down — otherwise `git worktree remove` would run against a detached,
  branch-borrowed state.
- **Shell `cd`:** focus prints `R`; an optional shell wrapper (mirroring the
  existing `wt shellinit` integration) can `cd` the user into the main repo so
  they can immediately run `yarn dev`.

## Edge cases & safety

- **Dirty main repo:** abort by default; `--autostash` to proceed. Never reset.
- **Dirty lender:** allowed; detaching preserves changes, re-attach restores them.
- **Interrupted focus (crash between git steps):** each command is idempotent
  and we roll back on failure; `ws status` surfaces a dangling session, and
  `ws unfocus --force` reconciles.
- **Branch already checked out in `R`:** if `R` is somehow already on `B`,
  report "already focused / branch in use" instead of swapping.
- **Remote workstreams (`Type == remote`):** unsupported — focus is a local
  worktree convenience; error early.

## Out of scope / alternatives considered

- **Copying or symlinking git-ignored artifacts into the worktree.** This avoids
  branch swapping entirely but means duplicating (or sharing, with subtle bugs)
  `node_modules`, build caches, and secrets across trees. Heavier and more
  error-prone than swapping the tracked content under the artifacts that already
  exist in the main repo. Not pursued, per the chosen direction.
- **A permanent parking branch per worktree.** See the mechanism decision above
  — rejected in favor of detached HEAD.
