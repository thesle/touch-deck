.PHONY: all build run clean install uninstall

# Install locations (override with e.g. `make install PREFIX=/usr/local`).
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin
ICONDIR ?= $(PREFIX)/share/icons/hicolor/256x256/apps
DESKTOPDIR ?= $(PREFIX)/share/applications

all: build

build:
	@echo "--- Building TouchDeck (Gio) ---"
	go build -o build/bin/touchdeck .
	@echo "TouchDeck binary created at: build/bin/touchdeck"

run:
	go run .

clean:
	@echo "--- Cleaning Build Output ---"
	rm -rf build/bin/touchdeck

# --- Desktop integration / app icon --------------------------------------
# WHY the .desktop + hicolor PNG mechanism:
# Gio v0.10.2 exposes NO window-icon API (there is no way to set the dock/panel
# icon from Go code). On Linux the panel/dock/application-menu icon is therefore
# resolved by the desktop environment from an installed XDG .desktop entry whose
# `Icon=touchdeck` key maps to a themed icon file -- here the hicolor 256x256 PNG
# installed to $(ICONDIR)/touchdeck.png. This installed .desktop + hicolor PNG
# pair is the AUTHORITATIVE icon path for this app; do not expect main.go to set
# a window icon (the API does not exist in this Gio version).
install: build
	install -Dm755 build/bin/touchdeck $(BINDIR)/touchdeck
	install -Dm644 build/appicon/touchdeck.png $(ICONDIR)/touchdeck.png
	install -Dm644 build/touchdeck.desktop $(DESKTOPDIR)/touchdeck.desktop
	sed -i "s|^Exec=.*|Exec=$(BINDIR)/touchdeck|" $(DESKTOPDIR)/touchdeck.desktop
	update-desktop-database $(DESKTOPDIR) 2>/dev/null || true
	gtk-update-icon-cache $(PREFIX)/share/icons/hicolor 2>/dev/null || true
	@echo "Installed TouchDeck to $(BINDIR)/touchdeck"

uninstall:
	rm -f $(BINDIR)/touchdeck
	rm -f $(ICONDIR)/touchdeck.png
	rm -f $(DESKTOPDIR)/touchdeck.desktop
	update-desktop-database $(DESKTOPDIR) 2>/dev/null || true
	gtk-update-icon-cache $(PREFIX)/share/icons/hicolor 2>/dev/null || true
	@echo "Uninstalled TouchDeck from $(BINDIR)/touchdeck"
