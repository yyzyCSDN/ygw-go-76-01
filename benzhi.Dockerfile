FROM golang:1.23

WORKDIR /app

COPY . .
RUN go build -mod=vendor ./...

ENV GOPROXY=off \
    GOSUMDB=off

CMD ["bash"]
