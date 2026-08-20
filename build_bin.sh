#!/usr/bin/env bash
# Build all golocate binaries from the repository root.
set -e
cd "$(dirname "$0")"
mkdir -p bin

echo "==> Build golocate (CLI)"
go build -o ./bin/golocate ./cmd/golocate/

echo "==> Build golocated (daemon)"
go build -o ./bin/golocated ./cmd/golocated/

# h5 is its own Go module; build it from inside ui/h5.
echo "==> Build golocate-h5 (H5 web bridge)"
(cd ui/h5 && go build -o ../../bin/golocate-h5 ./cmd/golocate-h5/)

echo "==> Build golocate-gtk (GTK GUI)"
if ! pkg-config --exists gtk4 2>/dev/null; then
    echo "GTK development libraries not found. Skipping GTK build."
    echo "To install GTK development libraries:"
    echo "  Ubuntu/Debian: sudo apt-get install libgtk-4-dev"
    echo "  Fedora/RHEL:   sudo dnf install gtk4-devel"
else
    # gotk4 is cgo-based; it is silently excluded when CGO is disabled.
    if [ "$(go env CGO_ENABLED)" != "1" ]; then
        echo "Note: CGO_ENABLED=$(go env CGO_ENABLED); gotk4 requires CGO — forcing CGO_ENABLED=1."
    fi
    if (cd ui/gtk && CGO_ENABLED=1 go build -o ../../bin/golocate-gtk ./cmd/golocate-gtk/); then
        echo "GTK version built successfully -> bin/golocate-gtk"
    else
        echo "ERROR: GTK build failed. gotk4 needs cgo plus GTK4/glib/GObject-Introspection"
        echo "development files. On Ubuntu/Debian:"
        echo "  sudo apt-get install libgtk-4-dev gobject-introspection \\"
        echo "                       libgirepository1.0-dev gir1.2-glib-2.0 golang-gocapability-dev"
        exit 1
    fi
fi

echo "==> Done: ./bin/{golocate,golocated,golocate-h5[,golocate-gtk]}"
