Build the stochadex documentation site.

This runs the docs build script at `docs/build.sh` which uses `pandoc` and `gomarkdoc` to generate HTML documentation from Go source comments and markdown files.

Steps:
1. Run `cd docs && bash build.sh` from the repo root.
2. If the build fails due to missing dependencies (`pandoc` or `gomarkdoc`), report which ones are missing and provide install instructions:
   - pandoc: `brew install pandoc` (macOS) or see https://pandoc.org/installing.html
   - gomarkdoc: `go install github.com/princjef/gomarkdoc/cmd/gomarkdoc@latest`
3. If the build succeeds, report which files were generated and confirm the build passed validation.
4. If $ARGUMENTS contains "clean", delete the generated files (`docs/pkg/`, `docs/index.html`, `docs/sitemap.xml`, `docs/robots.txt`) without rebuilding.
