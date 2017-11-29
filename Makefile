all: bat-go-docker

.PHONY: all bat-go-linux bat-go-docker

bat-go-linux:
	CGO_ENABLED=0 GOOS=linux go build -v -installsuffix cgo -o bat-go-linux .

bat-go-docker: bat-go-linux
	docker build -t bat-go .
