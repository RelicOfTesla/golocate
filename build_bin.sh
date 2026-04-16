mkdir -p bin
go build -o ./bin/golocate cmd/golocate/main.go
go build -o ./bin/golocated cmd/golocated/main.go
go build -o ./bin/golocate-h5 ui/h5/cmd/golocate-h5/main.go

# Check if GTK development libraries are installed
if pkg-config --exists gtk+-3.0 2>/dev/null; then
    go build -o ./bin/golocate-gtk ui/gtk/cmd/golocate-gtk/main.go
    echo "GTK version built successfully"
else
    echo "GTK development libraries not found. Skipping GTK build."
    echo "To install GTK development libraries:"
    echo "  Ubuntu/Debian: sudo apt-get install libgtk-3-dev"
    echo "  Fedora/RHEL: sudo dnf install gtk3-devel"
fi

