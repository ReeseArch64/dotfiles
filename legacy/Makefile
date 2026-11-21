BIN := bin/dotfiles

.PHONY: cli install run clean

cli:
	mkdir -p $(dir $(BIN))
	go build -trimpath -ldflags "-s -w" -o $(BIN) ./cmd/dotfiles

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
