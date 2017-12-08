FROM alpine:3.6
RUN apk add --update ca-certificates # Certificates for SSL
COPY target/linux_amd64/grant-server /bin/
EXPOSE 3333
CMD ["/bin/grant-server"]
