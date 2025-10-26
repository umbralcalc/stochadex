protoc -I=. \
    --go_out=$(pwd) \
    --js_out=library=./cmd/messages/partition_state_pb,binary:. \
    ./cmd/messages/partition_state.proto;
protoc --python_out=. ./cmd/messages/partition_state.proto;