all: bat-go-docker

.PHONY: all bat-go-linux bat-go-docker

GIT_VERSION := $(shell git describe --abbrev=8 --dirty --always --tags)

bat-go-linux:
	CGO_ENABLED=0 GOOS=linux go build -v -installsuffix cgo -o bat-go-linux .

bat-go-docker: bat-go-linux
	docker build -t bat-go:latest .
	docker tag bat-go:latest bat-go:$(GIT_VERSION)
