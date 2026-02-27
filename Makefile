BINARY := eg
AGENT_BINARY := eg-agent
MODULE := github.com/entryguard-io/cli

.PHONY: build build-agent install clean

build:
	go build -o $(BINARY) .

build-agent:
	go build -o $(AGENT_BINARY) ./cmd/eg-agent

install:
	go install .

clean:
	rm -f $(BINARY) $(AGENT_BINARY)
