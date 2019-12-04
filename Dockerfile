FROM golang:1.13-stretch as builder

SHELL ["/bin/bash", "-c"]

WORKDIR /src

COPY ./ /src/

RUN go test ./...

RUN go build

FROM golang:1.13-stretch

COPY --from=builder /src/nlb-attacher /nlb-attacher

CMD /nlb-attacher
