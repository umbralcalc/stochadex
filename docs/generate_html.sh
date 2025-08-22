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

# ...and generate package pages
for pkg in $(go list ../... | grep '/pkg/'); do
    out=pkg/$(basename $pkg).html
    godoc -url=/pkg/$pkg/ > "$out"
    pandoc "$out" -o "$out" --template=template.html
done