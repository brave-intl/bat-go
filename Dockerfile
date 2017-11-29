FROM alpine:3.6
RUN apk add --update ca-certificates # Certificates for SSL
COPY bat-go-linux /bin/bat-go
EXPOSE 3333
CMD ["/bin/bat-go"]
