# TouchDeck

## About

TouchDeck is a native desktop application built with [Gio](https://gioui.org/).
It presents a configurable grid of touch buttons that run shell commands, with a
built-in configuration editor. Config and images are stored under
`~/.config/touchdeck/`.

## Building

Build the binary with the Makefile or the Go toolchain directly:

```sh
make build      # produces build/bin/touchdeck
# or
go build .
```

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
