FROM golang:1.15.6-alpine
COPY . /src
WORKDIR /src
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/srpmproc

FROM centos:8.3.2011
COPY --from=0 /src/srpmproc /usr/bin/srpmproc

ENTRYPOINT ["/usr/bin/srpmproc"]
