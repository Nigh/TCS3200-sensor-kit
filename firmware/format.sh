#!/usr/bin/env bash
set -euo pipefail
find src tests -type f \( -name '*.c' -o -name '*.h' \) -print0 | xargs -0 clang-format -style=file -i
