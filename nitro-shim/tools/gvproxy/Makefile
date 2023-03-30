binary = gvproxy
godeps = go.mod go.sum *.go Makefile

.PHONY: all
all: lint $(binary)

.PHONY: lint
lint: $(godeps)
	golangci-lint run

$(binary): $(godeps)
	CGO_ENABLED=0 go build -o $(binary) .

.PHONY: clean
clean:
	rm -f $(binary)
