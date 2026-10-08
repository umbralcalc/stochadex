#!/usr/bin/env bash
#
# generate_proto.sh regenerates the message bindings the engine's websockets
# use, in every language their clients are written in:
#
#   - PartitionState (partition_state.proto), what a run streams out:
#       Go     -> pkg/simulator/partition_state.pb.go   (marshalled by the websocket
#                output function in pkg/simulator/output.go)
#       JS     -> cmd/messages/partition_state_pb.js     (browser websocket clients)
#       Python -> cmd/messages/partition_state_pb2.py    (Python websocket clients)
#   - ActionState (action_state.proto), what a client sends in (decoded by a
#     stream input with decode: protobuf_action_state, pkg/api/streams.go):
#       Go     -> pkg/simulator/action_state.pb.go
#       JS     -> cmd/messages/action_state_pb.js
#       Python -> cmd/messages/action_state_pb2.py
#
# Run from the repository root — the paths below are repo-root-relative:
#
#   bash cmd/messages/generate_proto.sh
#
# Requires `protoc` with the Go plugin (protoc-gen-go) on PATH, plus protoc's
# JS and Python generators. After editing a .proto, re-run this script and commit
# the regenerated files (never hand-edit them).

protoc -I=. \
    --go_out=$(pwd) \
    --js_out=library=./cmd/messages/partition_state_pb,binary:. \
    ./cmd/messages/partition_state.proto;
protoc --python_out=. ./cmd/messages/partition_state.proto;
protoc -I=. \
    --go_out=$(pwd) \
    --js_out=library=./cmd/messages/action_state_pb,binary:. \
    ./cmd/messages/action_state.proto;
protoc --python_out=. ./cmd/messages/action_state.proto;
