protoc -I=. \
    --go_out=$(pwd) \
    ./cmd/messages/partition_state.proto