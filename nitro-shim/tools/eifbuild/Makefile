binary = eifbuild
godeps = go.mod go.sum *.go

.PHONY: all test lint $(binary) clean

all: test lint $(binary)

$(binary): $(godeps)
	go build -o $(binary) .

clean:
	rm -f $(binary)
