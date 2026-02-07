#!/bin/bash

# Simplified build script for Stochadex documentation
# This script builds a professional documentation site without Python dependencies

set -e  # Exit on any error

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOCS_DIR="$SCRIPT_DIR"
PUBLIC_DIR="$DOCS_DIR"
TEMP_DIR="$DOCS_DIR/.temp"

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

# Generate HTML pages
generate_html_pages() {
    log_info "Generating HTML pages..."
    
    # Generate home page
    log_info "Generating home page..."
    pandoc --template "$DOCS_DIR/template.html" \
        --wrap=preserve \
        --citeproc \
        --csl="$DOCS_DIR/ieee.csl" \
        --bibliography="$DOCS_DIR/biblio.bib" \
        --mathjax \
        --syntax-highlighting=pygments \
        --metadata="is-home:true" \
        -f markdown \
        -t html \
        -o "$DOCS_DIR/index.html" \
        "$DOCS_DIR/README.md"
    
    # Generate quickstart page
    if [ -f "$DOCS_DIR/quickstart.md" ]; then
        log_info "Generating quickstart page..."
        local title=$(grep -E '^title:' "$DOCS_DIR/quickstart.md" | head -1 | sed 's/title: *"\(.*\)"/\1/' || echo "Quickstart")
        pandoc --template "$DOCS_DIR/template.html" \
            --wrap=preserve \
            --citeproc \
            --csl="$DOCS_DIR/ieee.csl" \
            --bibliography="$DOCS_DIR/biblio.bib" \
            --mathjax \
            --syntax-highlighting=pygments \
            --metadata="title:$title" \
            -f markdown \
            -t html \
            -o "$DOCS_DIR/pkg/quickstart.html" \
            "$DOCS_DIR/quickstart.md"
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
        
        # Fix headings and add metadata
        sed -i '' 's#</a>#</a>\
#g' "$TEMP_DIR/${pkg_name}.md"
        
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
            --template="$DOCS_DIR/template.html" \
            --mathjax \
            --syntax-highlighting=pygments
    done
    
    log_success "Package documentation generated"
}

# Generate sitemap
generate_sitemap() {
    log_info "Generating sitemap..."
    
    local base_url="https://umbralcalc.github.io/stochadex"
    
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

Sitemap: https://umbralcalc.github.io/stochadex/sitemap.xml
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
    
    # Check for broken anchor links (verify href="#id" targets exist in the same file)
    # Pandoc lowercases IDs and may add prefixes (e.g., func-, type-), so we check
    # case-insensitively whether the anchor appears as a suffix of any id in the file.
    for html_file in "$DOCS_DIR"/*.html "$DOCS_DIR"/pkg/*.html; do
        if [ ! -f "$html_file" ]; then
            continue
        fi
        local ids=$(grep -oE 'id="[^"]+' "$html_file" | sed 's/id="//' | tr '[:upper:]' '[:lower:]' | sort -u)
        local anchors=$(grep -oE 'href="#[^"]+' "$html_file" | sed 's/href="#//' | sort -u)
        for anchor in $anchors; do
            local anchor_lower=$(echo "$anchor" | tr '[:upper:]' '[:lower:]')
            if ! echo "$ids" | grep -q "$anchor_lower"; then
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
    generate_html_pages
    generate_package_docs
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
