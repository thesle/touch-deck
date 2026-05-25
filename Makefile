.PHONY: all dev build clean

all: build

dev:
	@echo "--- Running TouchDeck in Dev Mode (WebKitGTK 4.1) ---"
	wails dev -tags webkit2_41

build:
	@echo "--- Building TouchDeck Production Binary (WebKitGTK 4.1) ---"
	wails build -tags webkit2_41
	@echo "TouchDeck binary created at: build/bin/touch-scripts"

clean:
	@echo "--- Cleaning Build Directories ---"
	rm -rf build/bin/touch-scripts
