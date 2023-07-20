protoc -I=. \
    --go_out=$(pwd) \
    --plugin=protoc-gen-ts=./app/node_modules/.bin/protoc-gen-ts \
    --ts_out=. \
    ./app/src/dashboard_state.proto