protoc -I=. \
    --go_out=$(pwd) \
    --plugin=protoc-gen-ts=./app/node_modules/.bin/protoc-gen-ts \
    --js_out=import_style=commonjs,binary:. \
    --ts_out=. \
    ./app/src/dashboard_state.proto