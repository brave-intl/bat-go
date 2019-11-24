FROM golang:1.13.4 as builder

WORKDIR /src/
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY .git ./.git
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.GitCommit=$(git rev-list -1 HEAD)" -o grant-server ./bin/grant-server
CMD ["go", "run", "bin/grant-server/main.go"]

FROM alpine:3.6
RUN apk add --update ca-certificates # Certificates for SSL
COPY --from=builder /src/grant-server /bin/
COPY --from=builder /src/migrations/ /migrations/
EXPOSE 3333
CMD ["/bin/grant-server"]
