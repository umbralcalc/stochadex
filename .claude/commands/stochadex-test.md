Build and test the stochadex project.

If $ARGUMENTS is provided, treat it as a package path or test filter:
- A package name like "continuous" means run `go test ./pkg/continuous/...`
- A full path like "./pkg/simulator/..." is used as-is
- "all" or empty means run all tests

Steps:
1. Run `go build ./...` to check that everything compiles. If this fails, read the relevant source files, diagnose the error, and suggest a fix.
2. Run the appropriate `go test` command (see above). Use `-count=1` to avoid cached results.
3. Report results clearly. If any tests fail, read the failing test file and the source it tests, then explain what went wrong and suggest a fix.
4. Summarise: number of packages tested, pass/fail counts.
