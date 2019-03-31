FROM golang:1.11 as builder
ENV GO111MODULE=on

ADD . /go/src/github.com/monder/assign-eip/
WORKDIR /go/src/github.com/monder/assign-eip/

RUN go get
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags '-extldflags "-static"' -o assign-eip .


FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /go/src/github.com/monder/assign-eip/assign-eip /assign-eip

WORKDIR /
ENTRYPOINT ["/assign-eip"]

