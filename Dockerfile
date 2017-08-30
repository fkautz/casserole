FROM golang:1
#RUN apk --update add git ca-certificates
RUN apt update && apt install net-tools -y
ENV CGO_ENABLED 0
WORKDIR /go/src/github.com/fkautz/casserole
COPY . .
RUN go-wrapper install
ENV UPSTREAM_SERVER http://www.example.com
ENV ETCD http://etcd:2379
EXPOSE 80 8000
#RUN mkdir data
CMD casserole server --address=0.0.0.0:80 --mirror-url=${UPSTREAM_SERVER} --peering-address=http://${HOSTNAME}:8080 --etcd=${ETCD} --passthrough='\.pom$,\.pom\.sha1$,\.xml$,\.xml\.sha1$'
