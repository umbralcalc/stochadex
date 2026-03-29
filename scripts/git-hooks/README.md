# Git hooks

## `post-commit` — template tracks published `main`

After each commit that changes **anything other than** `template/go.mod` / `template/go.sum`, the hook runs:

```bash
cd template && go get github.com/umbralcalc/stochadex@main && go mod tidy
```

If `go.mod` / `go.sum` change, it creates a follow-up commit:

`chore(template): bump stochadex to main`

This keeps the template on the **latest stochadex revision visible to the Go proxy** for `main` (not your unpushed local `HEAD`). If `go get` fails (offline, proxy lag), the hook prints a message and **does not** fail your original commit.

### Install (once per clone)

```bash
git config core.hooksPath scripts/git-hooks
chmod +x scripts/git-hooks/post-commit
```

### Skip once

```bash
SKIP_TEMPLATE_BUMP_HOOK=1 git commit ...
```

### Uninstall

```bash
git config --unset core.hooksPath
```
