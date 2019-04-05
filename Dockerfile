FROM golang:1.11-alpine3.9
RUN apk --no-cache add git ca-certificates
ADD . /go/src/github.com/flant/addon-operator
RUN go get -d github.com/flant/addon-operator/...
WORKDIR /go/src/github.com/flant/addon-operator
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o addon-operator ./cmd/addon-operator

FROM ubuntu:18.04
RUN apt-get update && \
    apt-get install -y ca-certificates wget jq && \
    rm -rf /var/lib/apt/lists && \
    wget https://storage.googleapis.com/kubernetes-release/release/v1.13.5/bin/linux/amd64/kubectl -O /bin/kubectl && \
    chmod +x /bin/kubectl && \
    mkdir /hooks
COPY --from=0 /go/src/github.com/flant/addon-operator/addon-operator /
WORKDIR /
ENV ADDON_OPERATOR_WORKING_DIR /addons
ENTRYPOINT ["/addon-operator"]
#CMD ["start"]
