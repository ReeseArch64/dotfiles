BIN := bin/dotfiles

.PHONY: cli install run clean

cli:
	go build -trimpath -ldflags "-s -w" -o $(BIN) ./cmd/dotfiles

# Symlink em ~/.local/bin: o binário acha o repositório seguindo o link.
install: cli
	mkdir -p $(HOME)/.local/bin
	ln -sf $(CURDIR)/$(BIN) $(HOME)/.local/bin/dotfiles

run: cli
	./$(BIN)

clean:
	rm -rf bin
