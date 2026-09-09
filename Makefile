.PHONY: all build run clean

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
