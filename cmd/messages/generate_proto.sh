#!/usr/bin/env bash
#
# generate_proto.sh regenerates the PartitionState message bindings from
# partition_state.proto, in every language the engine's streaming output targets:
#
#   - Go     -> pkg/simulator/partition_state.pb.go   (marshalled by the websocket
#              output function in pkg/simulator/output.go)
#   - JS     -> cmd/messages/partition_state_pb.js     (browser websocket clients)
#   - Python -> cmd/messages/partition_state_pb2.py    (Python websocket clients)
#
# Run from the repository root — the paths below are repo-root-relative:
#
#   bash cmd/messages/generate_proto.sh
#
# Requires `protoc` with the Go plugin (protoc-gen-go) on PATH, plus protoc's
# built-in JS and Python generators. After editing partition_state.proto, re-run
# this script and commit the regenerated files (never hand-edit them).

protoc -I=. \
    --go_out=$(pwd) \
    --js_out=library=./cmd/messages/partition_state_pb,binary:. \
    ./cmd/messages/partition_state.proto;
protoc --python_out=. ./cmd/messages/partition_state.proto;