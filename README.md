# TouchDeck

## About

TouchDeck is a native desktop application built with [Gio](https://gioui.org/).
It presents a configurable grid of touch buttons that run shell commands, with a
built-in configuration editor. Config and images are stored in
`touchdeck/` under the OS user config directory: `~/.config/touchdeck/` on
Linux and `%AppData%\touchdeck\` on Windows.

Button commands run through `bash -c` on Linux and `cmd.exe /d /s /c` on
Windows (without opening a console window).

## Building

Build the binary with the Makefile or the Go toolchain directly:

```sh
make build      # produces build/bin/touchdeck
# or
go build .
```

## Windows

Cross-compile the Windows binary from Linux with:

```sh
make build-windows   # produces build/bin/touchdeck.exe
```

This builds a GUI-subsystem exe (`-ldflags="-H windowsgui"`), so no console
window opens with the app. A plain `go build .` on Windows produces a
console-subsystem exe that opens a console window alongside the app.

## Running

```sh
make run
# or
go run .
```

## Linux build dependencies

Gio and the native file dialog need the following system packages:

- `libvulkan-dev`
- `libxkbcommon-x11-dev`
- `libx11-xcb-dev`
- GTK development libraries (for the native file-open dialog)
