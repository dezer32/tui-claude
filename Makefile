BINARY := tui-claude
PKG := github.com/vladislav-k/tui-claude

.PHONY: build run clean install

build:
	go build -o $(BINARY) .

run: build
	./$(BINARY)

install: build
	cp $(BINARY) $(GOPATH)/bin/ 2>/dev/null || cp $(BINARY) ~/go/bin/

clean:
	rm -f $(BINARY)

tmux-run: build
	tmux new-session -s tui-claude "./$(BINARY)" 2>/dev/null || tmux attach -t tui-claude
