FROM alpine:latest

RUN mkdir /lib64 && mkdir /app && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2
COPY ./docker-dns /app/docker-dns

EXPOSE 5300
WORKDIR /app
ENTRYPOINT ["./docker-dns","-d"]