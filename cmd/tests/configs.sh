#!/bin/bash

TEST_CONFIGS=(
    "./cfg/example_config.yaml"
    "./cfg/example_inference_config.yaml"
)

go build -o bin/ ./cmd/stochadex
if [[ $? -ne 0 ]]; then
    echo "Compilation failed."
    exit 1
fi

all_tests_passed=true
for config_file in "${TEST_CONFIGS[@]}"; do
    echo "Test run of config: $config_file"
    stderr_output=$(./bin/stochadex --config "$config_file" 2>&1 >/dev/null)

    if [[ -z "$stderr_output" ]]; then
        echo "************PASS*************"
    else
        echo "************FAIL*************"
        echo "stderr output:"
        echo "$stderr_output"
        all_tests_passed=false
    fi
done

if $all_tests_passed; then
    echo "**********ALL PASS***********"
else
    echo "************FAIL*************"
fi