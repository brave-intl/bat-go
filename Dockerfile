FROM golang:1.15 as builder

ARG VERSION
ARG BUILD_TIME
ARG COMMIT

WORKDIR /src/
COPY go.mod go.sum ./
RUN go mod download
COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-w -s -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.commit=${COMMIT}" \
    -o grant-server ./bin/grant-server

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-w -s -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.commit=${COMMIT}" \
    -o bat-go main.go

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-w -s -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.commit=${COMMIT}" \
    -o settlement-submit ./bin/settlement-submit

FROM alpine:3.6 as artifact
RUN apk add --update ca-certificates # Certificates for SSL
COPY --from=builder /src/grant-server /bin/
COPY --from=builder /src/bat-go /bin/
COPY --from=builder /src/settlement-submit /bin/
COPY --from=builder /src/migrations/ /migrations/
EXPOSE 3333
CMD ["/bin/grant-server"]
