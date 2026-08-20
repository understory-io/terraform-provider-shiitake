#!/usr/bin/env bash
# Refresh the vendored shiitake service contract and regenerate the Go client.
#
# The protos are vendored rather than pulled from the BSR: buf.build/understory-io/shiitake
# is declared in shiitake's buf.yaml but is not published, so `buf generate`
# against it fails on a clean machine. Vendoring keeps the provider buildable
# from a fresh checkout; this script is how the copy stays honest.
#
# Usage: scripts/vendor-proto.sh [path-to-shiitake-checkout]
set -euo pipefail

SHIITAKE="${1:-../shiitake}"
SRC="$SHIITAKE/interface/proto/shiitake/v1"

if [ ! -d "$SRC" ]; then
  echo "no shiitake protos at $SRC — pass the checkout path as \$1" >&2
  exit 1
fi

cp "$SRC/schedule.proto" proto/shiitake/v1/schedule.proto

# Re-add the go_package option. Upstream does not carry one (its codegen is
# Rust-only), and buf needs it to place the generated package.
python3 scripts/add_go_package.py

buf generate
echo "vendored + regenerated. Review \`git diff\` — a changed RPC is a provider change."
