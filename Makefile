GIT_VERSION := $(shell git describe --abbrev=8 --dirty --always --tags)
_BINS := $(wildcard bin/*)
ifdef GOOS
	BINS := $(_BINS:bin/%=target/$(GOOS)_$(GOARCH)/%)
else
	BINS := $(_BINS:bin/%=%)
endif

.PHONY: all bins docker test lint clean
all: test bins
	
bins: clean $(BINS)

.DEFAULT:
	go build ./bin/$@

target/%:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $@ ./bin/$(notdir $@) 

docker:
	GOOS=linux GOARCH=amd64 make bins
	docker build -t bat-go:latest .
	docker tag bat-go:latest bat-go:$(GIT_VERSION)

test:
	go test -v --tags=$(TEST_TAGS) ./...

lint:
	gometalinter --vendor --disable=gocyclo --deadline=2m ./...

clean:
	rm -f $(BINS)
