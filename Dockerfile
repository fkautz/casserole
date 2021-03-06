FROM registry.access.redhat.com/ubi8/ubi as build
#RUN apk --update add git ca-certificates
RUN yum update && yum install -y golang
ENV CGO_ENABLED 0
WORKDIR /go/src/github.com/fkautz/casserole

COPY go.mod go.sum ./
COPY ./pkg/imports/ ./pkg/imports/
RUN go build ./pkg/imports/

COPY . .
RUN go install -v ./cmd/casserole
ENV UPSTREAM_SERVER http://www.example.com
ENV ETCD http://etcd:2379
EXPOSE 80 8000

FROM registry.access.redhat.com/ubi8/ubi-minimal
ENV CASSEROLE_ADDRESS 0.0.0.0:80
ENV CASSEROLE_MIRRORURL http://www.google.com
ENV CASSEROLE_ETCD http://etcd:2379
ENV CASSEROLE_PASSTHROUGH '\.pom$,\.pom\.sha1$,\.xml$,\.xml\.sha1$'
COPY --from=build /root/go/bin/casserole /usr/local/bin/casserole
RUN mkdir data

CMD CASSEROLE_PEERINGADDRESS=http://${HOSTNAME}:8080 /usr/local/bin/casserole server

