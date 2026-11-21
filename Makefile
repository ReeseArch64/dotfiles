BIN := bin/dotfiles

.PHONY: cli test install run clean

cli:
	mkdir -p $(dir $(BIN))
	cd cli && go build -trimpath -ldflags "-s -w" -o "$(abspath $(BIN))" .

test:
	cd cli && go test ./...

install: cli
	mkdir -p "$$HOME/.local/bin"
	@set -e; \
	staged=$$(mktemp "$$HOME/.local/bin/.dotfiles.XXXXXXXX"); \
	trap 'rm -f -- "$$staged"' EXIT; \
	install -m 0755 "$(BIN)" "$$staged"; \
	mv -fT -- "$$staged" "$$HOME/.local/bin/dotfiles"

run: cli
	./$(BIN)

clean:
	rm -rf bin
