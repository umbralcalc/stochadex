#!/bin/bash

# generate about page
pandoc --template template.html \
--wrap=preserve \
--citeproc \
--csl=ieee.csl \
--bibliography=biblio.bib \
--mathjax \
-f markdown \
-t html \
-o index.html \
README.md;

# generate other pages
for filename in _pages/*.md; do
    prefix="_pages/"
    suffix=".md"
    newFilename=${filename%"$suffix"}
    newFilename=${newFilename#"$prefix"}
    pandoc --template template.html \
    --wrap=preserve \
    --citeproc \
    --csl=ieee.csl \
    --bibliography=biblio.bib \
    --mathjax \
    -f markdown \
    -t html \
    -o pages/${newFilename}.html \
    ${filename};
done

# ...and generate package pages using gomarkdoc
# (go install github.com/princjef/gomarkdoc/cmd/gomarkdoc@latest)
for pkg in $(go list ../... | grep '/pkg/'); do
    out=$(basename $pkg)
    gomarkdoc $pkg --output _pkg/$out.md
    # hack fixes headings
    sed -i 's#</a>#</a>\n#g' _pkg/$out.md
    pandoc _pkg/$out.md -o pkg/$out.html --template=template.html
done