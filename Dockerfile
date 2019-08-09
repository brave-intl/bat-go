FROM golang:1.12 as builder

WORKDIR /src/
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -o grant-server ./bin/grant-server
CMD ["go", "run", "bin/grant-server/main.go"]

FROM alpine:3.6
RUN apk add --update ca-certificates # Certificates for SSL
COPY --from=builder /src/grant-server /bin/
COPY --from=builder /src/migrations/ /migrations/
EXPOSE 3333
CMD ["/bin/grant-server"]
