binary = viproxy
godeps = go.mod go.sum *.go

.PHONY: all test lint $(binary) clean

all: test lint $(binary)

test: $(godeps)
	go test -cover ./...

lint: $(godeps)
	golangci-lint run ./...

$(binary): $(godeps)
	go build -o $(binary) .

clean:
	rm -f $(binary)
