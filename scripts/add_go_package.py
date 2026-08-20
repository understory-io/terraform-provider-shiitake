"""Add the go_package option to the vendored shiitake protos.

Upstream (understory-io/shiitake) carries no go_package: its codegen is
Rust-only, so nothing there needs one. buf does, to place the generated Go
package. Kept as a script rather than a heredoc so vendor-proto.sh stays
readable and this stays testable.
"""

import pathlib

GO_PACKAGE = (
    "github.com/understory-io/terraform-provider-shiitake/gen/shiitake/v1;shiitakev1"
)

NOTE = (
    "package shiitake.v1;\n\n"
    "// Vendored from understory-io/shiitake (interface/proto). Kept byte-identical\n"
    "// apart from this go_package option, which the Rust codegen does not need.\n"
    f'option go_package = "{GO_PACKAGE}";'
)


def main() -> None:
    for path in pathlib.Path("proto").rglob("*.proto"):
        text = path.read_text()
        if "go_package" in text:
            continue
        if "package shiitake.v1;" not in text:
            raise SystemExit(f"{path}: unexpected package declaration")
        path.write_text(text.replace("package shiitake.v1;", NOTE, 1))
        print(f"added go_package to {path}")


if __name__ == "__main__":
    main()
