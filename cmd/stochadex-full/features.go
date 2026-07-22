package main

// features lists the optional capabilities compiled into this binary. Build-tagged files
// append to it at init, so `stochadex --version` reports exactly what this executable can
// do — the question an agent (or a user) otherwise has no way to answer, since the
// portable and accelerated assets share a name and a CLI.
var features = []string{"arrow", "postgres"}
