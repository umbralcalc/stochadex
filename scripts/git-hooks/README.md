# Git hooks (stochadex repo)

## Template module auto-bump (`post-commit`)

After each commit that changes anything other than `template/go.mod` / `template/go.sum`, the hook pins `template/` to the current repository `HEAD` and, if `go.mod` / `go.sum` change, creates a follow-up commit:

`chore(template): bump stochadex module to HEAD`

So the template bump stays the last commit in the chain.

### Install (once per clone)

From the repository root:

```bash
git config core.hooksPath scripts/git-hooks
chmod +x scripts/git-hooks/post-commit
```

`core.hooksPath` is stored in this repo’s local `.git/config` (not committed). Repeat on new machines or after cloning.

### Skip for one commit

```bash
SKIP_TEMPLATE_BUMP_HOOK=1 git commit ...
```

### Requirements

- `go` on `PATH`.
- After `dropreplace`, `go mod tidy` may need network access to refresh `go.sum` for the new pseudo-version (or ensure `HEAD` is available to your module proxy).
