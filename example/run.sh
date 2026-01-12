#!/bin/sh
set -u

src_arg="${1:-}"

compile_log="$(clang++ -march=native -O3 -flto -std=c++23 -o eval eval.cpp "${src_arg}" 2>&1)"
compile_rc=$?

if [[ $compile_rc -ne 0 ]]; then
    echo "-1"
    echo "compilation error: ${compile_log}"
    exit 0
fi

tmp_out="$(mktemp)"
tmp_rc="$(mktemp)"
trap 'rm -f "$tmp_out" "$tmp_rc"' EXIT

timeout --preserve-status 60s ./eval >"$tmp_out" 2>&1
run_rc=$?
run_out="$(cat "$tmp_out")"

if [[ $run_rc -eq 124 ]]; then
    echo "-1"
    echo "time limit exceeded"
    exit 0
fi

if [[ $run_rc -ne 0 ]]; then
    echo "-1"
    echo "runtime error: ${run_out}"
    exit 0
fi

printf '%s\n' "$run_out"
echo "ok"
exit 0
