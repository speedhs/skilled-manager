# Roadmap

Not in this release. Recorded so they are not built accidentally.

- Adopt target-side edits back into the canonical skill ("take theirs") as an explicit command, separate from sync.
- Deploy one skill to several scopes (project `.claude/skills`, repo `.agents/skills`) in addition to the user scope.
- Optional opt-in update check. Rejected for now because the CLI must not make network calls.
- macOS Developer ID notarization and Windows Authenticode once certificates exist.
- Publish the Homebrew tap and Scoop bucket instead of only generating the manifests.
- Skill rename / delete from the GUI, with the same delete rules as disable.
- Merge dialog when the editor and disk both changed, instead of reload-or-keep.
- Import from a git URL. That is a network feature and stays out until the no-network rule is revisited.
