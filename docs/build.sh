#!/bin/bash

# Simplified build script for Stochadex documentation
# This script builds a professional documentation site without Python dependencies

set -e  # Exit on any error

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOCS_DIR="$SCRIPT_DIR"
PUBLIC_DIR="$DOCS_DIR"
TEMP_DIR="$DOCS_DIR/.temp"
MODELS_DIR="$DOCS_DIR/../models"
WORK_TEMPLATE="$TEMP_DIR/template.html"
REPO_BLOB_URL="https://github.com/umbralcalc/stochadex/blob/main/models"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check dependencies
check_dependencies() {
    log_info "Checking dependencies..."
    
    local missing_deps=()
    
    if ! command -v pandoc &> /dev/null; then
        missing_deps+=("pandoc")
    fi
    
    if ! command -v gomarkdoc &> /dev/null; then
        missing_deps+=("gomarkdoc")
    fi
    
    if [ ${#missing_deps[@]} -ne 0 ]; then
        log_error "Missing dependencies: ${missing_deps[*]}"
        log_info "Install missing dependencies:"
        for dep in "${missing_deps[@]}"; do
            case $dep in
                "pandoc")
                    echo "  - pandoc: https://pandoc.org/installing.html"
                    ;;
                "gomarkdoc")
                    echo "  - gomarkdoc: go install github.com/princjef/gomarkdoc/cmd/gomarkdoc@latest"
                    ;;
            esac
        done
        exit 1
    fi
    
    log_success "All dependencies found"
}

# Clean previous build
clean_build() {
    log_info "Cleaning previous build..."
    
    # Remove only generated HTML files, not source files
    if [ -d "$DOCS_DIR/pkg" ]; then
        rm -rf "$DOCS_DIR/pkg"
    fi
    
    if [ -f "$DOCS_DIR/index.html" ]; then
        rm -f "$DOCS_DIR/index.html"
    fi
    
    if [ -f "$DOCS_DIR/sitemap.xml" ]; then
        rm -f "$DOCS_DIR/sitemap.xml"
    fi
    
    if [ -f "$DOCS_DIR/robots.txt" ]; then
        rm -f "$DOCS_DIR/robots.txt"
    fi
    
    if [ -d "$TEMP_DIR" ]; then
        rm -rf "$TEMP_DIR"
    fi
    
    mkdir -p "$TEMP_DIR"
    
    log_success "Build directory cleaned"
}

# Copy static assets
copy_assets() {
    log_info "Copying static assets..."
    
    # Assets are already in the right place, just ensure they exist
    if [ -d "$DOCS_DIR/assets" ]; then
        log_success "Assets directory found"
    else
        log_warning "Assets directory not found"
    fi
}

# Prepare a working copy of the template with the Models nav list injected.
# The source template.html carries empty MODELS_NAV markers; this fills them from
# whatever lives in models/ so /new-model entries appear in the sidebar automatically.
prepare_template() {
    log_info "Preparing navigation template..."

    local packages_nav="$TEMP_DIR/packages_nav.html"
    local models_nav="$TEMP_DIR/models_nav.html"
    : > "$packages_nav"
    : > "$models_nav"

    # Packages: auto-discover from the module's pkg/ tree — the same source the package
    # docs are generated from — so a new package appears in the sidebar automatically.
    for pkg in $(go list ../... | grep '/pkg/'); do
        local pkg_name=$(basename "$pkg")
        case "$pkg_name" in
            api) local label="API" ;;                            # known acronym
            *)   local label=$(echo "$pkg_name" \
                     | awk '{print toupper(substr($0,1,1)) substr($0,2)}') ;;
        esac
        printf '        <li><a href="$if(is-home)$$else$../$endif$pkg/%s.html">%s</a></li>\n' \
            "$pkg_name" "$label" >> "$packages_nav"
    done

    # Domain models: auto-discover from the models/ catalogue.
    for card in "$MODELS_DIR"/*/card.md; do
        [ -f "$card" ] || continue
        local name=$(basename "$(dirname "$card")")
        # Short sidebar label: dir name, hyphens to spaces, Title Cased.
        local label=$(echo "$name" | tr '-' ' ' \
            | awk '{for(i=1;i<=NF;i++) $i=toupper(substr($i,1,1)) substr($i,2)}1')
        printf '        <li><a href="$if(is-home)$$else$../$endif$pkg/model-%s.html">%s</a></li>\n' \
            "$name" "$label" >> "$models_nav"
    done

    # Splice each nav list between its markers into the working template.
    awk -v pkgfile="$packages_nav" -v modfile="$models_nav" '
        /<!-- PACKAGES_NAV_START -->/ {
            print
            while ((getline line < pkgfile) > 0) print line
            close(pkgfile)
            skip=1
            next
        }
        /<!-- PACKAGES_NAV_END -->/ { skip=0; print; next }
        /<!-- MODELS_NAV_START -->/ {
            print
            while ((getline line < modfile) > 0) print line
            close(modfile)
            skip=1
            next
        }
        /<!-- MODELS_NAV_END -->/ { skip=0; print; next }
        skip==1 { next }
        { print }
    ' "$DOCS_DIR/template.html" > "$WORK_TEMPLATE"

    log_success "Navigation template prepared ($(grep -c '<li>' "$packages_nav") packages, $(grep -c '<li>' "$models_nav") models)"
}

# Generate HTML pages
generate_html_pages() {
    log_info "Generating HTML pages..."
    
    # Generate home page
    log_info "Generating home page..."
    pandoc --template "$WORK_TEMPLATE" \
        --wrap=preserve \
        --mathjax \
        --highlight-style=pygments \
        --metadata="is-home:true" \
        -f markdown \
        -t html \
        -o "$DOCS_DIR/index.html" \
        "$DOCS_DIR/README.md"
    
    # Generate quickstart page
    if [ -f "$DOCS_DIR/quickstart.md" ]; then
        log_info "Generating quickstart page..."
        local title=$(grep -E '^title:' "$DOCS_DIR/quickstart.md" | head -1 | sed 's/title: *"\(.*\)"/\1/' || echo "Quickstart")
        pandoc --template "$WORK_TEMPLATE" \
            --wrap=preserve \
            --mathjax \
            --highlight-style=pygments \
            --metadata="title:$title" \
            -f markdown \
            -t html \
            -o "$DOCS_DIR/pkg/quickstart.html" \
            "$DOCS_DIR/quickstart.md"
    fi

    # Generate how it works page
    if [ -f "$DOCS_DIR/how_it_works.md" ]; then
        log_info "Generating how it works page..."
        local title=$(grep -E '^title:' "$DOCS_DIR/how_it_works.md" | head -1 | sed 's/title: *"\(.*\)"/\1/' || echo "How it works")
        pandoc --template "$WORK_TEMPLATE" \
            --wrap=preserve \
            --mathjax \
            --highlight-style=pygments \
            --metadata="title:$title" \
            -f markdown \
            -t html \
            -o "$DOCS_DIR/pkg/how_it_works.html" \
            "$DOCS_DIR/how_it_works.md"
    fi
    
    log_success "HTML pages generated"
}

# Generate package documentation
generate_package_docs() {
    log_info "Generating package documentation..."
    
    # Create pkg directory
    mkdir -p "$DOCS_DIR/pkg"
    
    # Generate package pages using gomarkdoc
    for pkg in $(go list ../... | grep '/pkg/'); do
        local pkg_name=$(basename "$pkg")
        local pkg_title=$(echo "$pkg_name" | sed 's/.*\///')
        
        log_info "Generating package: $pkg_name"
        
        # Generate markdown with better formatting
        gomarkdoc "$pkg" --output "$TEMP_DIR/${pkg_name}.md" --format github --verbose
        
        # Fix headings and add metadata. Use perl (portable across BSD/GNU) to add a
        # newline after each </a> — GNU sed rejects the BSD `sed -i ''` in-place form.
        perl -0777 -i -pe 's#</a>#</a>\n#g' "$TEMP_DIR/${pkg_name}.md"
        
        # Post-process to fix Example code blocks in docstrings
        # Only convert opening ``` that are not already followed by a language tag
        # Use awk to be more precise about which code blocks to convert
        awk '
        BEGIN { in_code_block = 0; }
        /^```$/ && !in_code_block { 
            # This is an opening code block without language tag
            in_code_block = 1; 
            print "```go";
            next;
        }
        /^```$/ && in_code_block { 
            # This is a closing code block
            in_code_block = 0; 
            print "```";
            next;
        }
        /^```[a-zA-Z]/ {
            # This is already a code block with language tag, leave it alone
            in_code_block = 1;
            print;
            next;
        }
        { print; }
        ' "$TEMP_DIR/${pkg_name}.md" > "$TEMP_DIR/${pkg_name}_processed.md"
        mv "$TEMP_DIR/${pkg_name}_processed.md" "$TEMP_DIR/${pkg_name}.md"
        
        # Add frontmatter
        cat > "$TEMP_DIR/${pkg_name}_with_meta.md" << EOF
---
title: "$pkg_title"
logo: true
---

$(cat "$TEMP_DIR/${pkg_name}.md")
EOF
        
        # Generate HTML
        pandoc "$TEMP_DIR/${pkg_name}_with_meta.md" \
            -o "$DOCS_DIR/pkg/${pkg_name}.html" \
            --template="$WORK_TEMPLATE" \
            --mathjax \
            --highlight-style=pygments
    done
    
    log_success "Package documentation generated"
}

# Generate model card pages from models/*/card.md
generate_model_docs() {
    log_info "Generating model card pages..."

    mkdir -p "$DOCS_DIR/pkg"

    for card in "$MODELS_DIR"/*/card.md; do
        [ -f "$card" ] || continue
        local name=$(basename "$(dirname "$card")")
        local title=$(grep -m1 -E '^# ' "$card" | sed 's/^# *//')
        [ -z "$title" ] && title="$name"

        log_info "Generating model: $name"

        # Rewrite sibling-file relative links (e.g. [`stub.go`](stub.go)) to GitHub blob
        # URLs so they resolve in the docs site. Only targets that are a bare filename
        # ending in .go/.md/.yaml/.yml are rewritten; paths with slashes are left alone.
        sed -E "s#\]\(([A-Za-z0-9_.-]+\.(go|md|ya?ml))\)#](${REPO_BLOB_URL}/${name}/\1)#g" \
            "$card" > "$TEMP_DIR/model-${name}.md"

        # Prepend frontmatter (title drives the <title> tag and page header)
        cat > "$TEMP_DIR/model-${name}_with_meta.md" << EOF
---
title: "$title"
logo: true
---

$(cat "$TEMP_DIR/model-${name}.md")
EOF

        pandoc "$TEMP_DIR/model-${name}_with_meta.md" \
            -o "$DOCS_DIR/pkg/model-${name}.html" \
            --template="$WORK_TEMPLATE" \
            --mathjax \
            --highlight-style=pygments
    done

    log_success "Model card pages generated"
}

# Post-process mermaid blocks. Pandoc renders ```mermaid as
# <pre class="mermaid"><code>…</code></pre>; mermaid.js expects the source directly
# under the .mermaid element, so strip the inner <code> wrapper. Only mermaid blocks
# are touched — other code blocks keep their <pre class="sourceCode …"><code> markup.
render_mermaid() {
    log_info "Post-processing mermaid diagrams..."

    local count=0
    for html_file in "$DOCS_DIR"/index.html "$DOCS_DIR"/pkg/*.html; do
        [ -f "$html_file" ] || continue
        if grep -q '<pre class="mermaid"><code>' "$html_file"; then
            perl -0777 -i -pe \
                's#<pre class="mermaid"><code>(.*?)</code></pre>#<pre class="mermaid">$1</pre>#gs' \
                "$html_file"
            # Docs palette: recolour past-history (cross-history) node fill. The graph
            # generator emits classDef pastcopy fill:#d8e6f3 (see pkg/graph/render.go);
            # mermaid applies it inline with !important, so it must be remapped in the
            # source here rather than via a stylesheet override.
            perl -0777 -i -pe 's/fill:#d8e6f3/fill:#F5F5F5/g' "$html_file"
            ((count++)) || true
        fi
    done

    log_success "Mermaid diagrams post-processed ($count pages)"
}

# Wrap each rendered <table> in <div class="table-scroll"> so wide tables scroll
# horizontally (styled in template.html) instead of squashing their columns. Pandoc
# does not emit a wrapper, and clean_build regenerates pages each run, so there is no
# risk of double-wrapping.
wrap_tables() {
    log_info "Wrapping tables for horizontal scroll..."

    for html_file in "$DOCS_DIR"/index.html "$DOCS_DIR"/pkg/*.html; do
        [ -f "$html_file" ] || continue
        perl -0777 -i -pe \
            's#(<table\b.*?</table>)#<div class="table-scroll">$1</div>#gs' \
            "$html_file"
    done

    log_success "Tables wrapped"
}

# Generate sitemap
generate_sitemap() {
    log_info "Generating sitemap..."
    
    local base_url="https://stochadex.github.io/"
    
    cat > "$DOCS_DIR/sitemap.xml" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>$base_url/</loc>
    <lastmod>$(date -u +%Y-%m-%d)</lastmod>
    <changefreq>weekly</changefreq>
    <priority>1.0</priority>
  </url>
EOF
    
    # Add quickstart page
    if [ -f "$DOCS_DIR/pkg/quickstart.html" ]; then
        cat >> "$DOCS_DIR/sitemap.xml" << EOF
  <url>
    <loc>$base_url/pkg/quickstart.html</loc>
    <lastmod>$(date -u +%Y-%m-%d)</lastmod>
    <changefreq>monthly</changefreq>
    <priority>0.9</priority>
  </url>
EOF
    fi
    
    # Add package docs
    for file in "$DOCS_DIR"/pkg/*.html; do
        if [ -f "$file" ]; then
            local filename=$(basename "$file")
            cat >> "$DOCS_DIR/sitemap.xml" << EOF
  <url>
    <loc>$base_url/pkg/$filename</loc>
    <lastmod>$(date -u +%Y-%m-%d)</lastmod>
    <changefreq>monthly</changefreq>
    <priority>0.6</priority>
  </url>
EOF
        fi
    done
    
    cat >> "$DOCS_DIR/sitemap.xml" << EOF
</urlset>
EOF
    
    log_success "Sitemap generated"
}

# Generate robots.txt
generate_robots() {
    log_info "Generating robots.txt..."
    
    cat > "$DOCS_DIR/robots.txt" << EOF
User-agent: *
Allow: /

Sitemap: https://stochadex.github.io/sitemap.xml
EOF
    
    log_success "robots.txt generated"
}

# Validate build
validate_build() {
    log_info "Validating build..."
    
    local errors=0
    
    # Check if main files exist
    if [ ! -f "$DOCS_DIR/index.html" ]; then
        log_error "index.html not found"
        ((errors++))
    fi
    
    if [ ! -d "$DOCS_DIR/assets" ]; then
        log_error "assets directory not found"
        ((errors++))
    fi
    
    # Check for broken anchor links (verify href="#id" targets exist in the same file).
    # Pandoc lowercases IDs and may add prefixes (e.g., func-, type-), so we check
    # case-insensitively whether the anchor appears as a substring of any id/name.
    # Targets are id="..." attributes AND <a name="..."> anchors (gomarkdoc emits the
    # latter, e.g. for generic methods like <a name="MCTSTree[S, A].AdvanceRoot">).
    # Hrefs are URL-decoded before comparison so %5B/%20 match the literal [/space
    # characters that gomarkdoc puts in name attributes.
    for html_file in "$DOCS_DIR"/*.html "$DOCS_DIR"/pkg/*.html; do
        if [ ! -f "$html_file" ]; then
            continue
        fi
        local targets=$( { grep -oE 'id="[^"]+' "$html_file" | sed 's/id="//'; \
                           grep -oE 'name="[^"]+' "$html_file" | sed 's/name="//'; } \
                         | tr '[:upper:]' '[:lower:]' | sort -u)
        local anchors=$(grep -oE 'href="#[^"]+' "$html_file" | sed 's/href="#//' | sort -u)
        for anchor in $anchors; do
            local anchor_decoded=$(printf '%b' "${anchor//%/\\x}")
            local anchor_lower=$(echo "$anchor_decoded" | tr '[:upper:]' '[:lower:]')
            if ! echo "$targets" | grep -qF "$anchor_lower"; then
                log_warning "Broken anchor link #$anchor in $(basename "$html_file")"
            fi
        done
    done
    
    if [ $errors -eq 0 ]; then
        log_success "Build validation passed"
    else
        log_error "Build validation failed with $errors errors"
        exit 1
    fi
}

# Main build function
main() {
    log_info "Starting simplified documentation build..."
    
    check_dependencies
    clean_build
    copy_assets
    prepare_template
    generate_html_pages
    generate_package_docs
    generate_model_docs
    render_mermaid
    wrap_tables
    generate_sitemap
    generate_robots
    validate_build
    
    # Clean up temporary files
    if [ -d "$TEMP_DIR" ]; then
        rm -rf "$TEMP_DIR"
        log_info "Cleaned up temporary files"
    fi
    
    log_success "Documentation build completed successfully!"
    log_info "Output directory: $DOCS_DIR"
    log_info "You can now serve the documentation with:"
    log_info "  cd $DOCS_DIR && python3 -m http.server 8000"
    log_info "  or"
    log_info "  cd $DOCS_DIR && npx serve ."
}

# Run main function
main "$@"
