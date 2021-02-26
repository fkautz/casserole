FROM registry.access.redhat.com/ubi8/ubi as build
#RUN apk --update add git ca-certificates
RUN yum update && yum install -y golang
ENV CGO_ENABLED 0
WORKDIR /go/src/github.com/fkautz/casserole
COPY . .
RUN go install -v
ENV UPSTREAM_SERVER http://www.example.com
ENV ETCD http://etcd:2379
EXPOSE 80 8000
RUN mkdir data

FROM registry.access.redhat.com/ubi8/ubi-minimal
ENV UPSTREAM_SERVER http://www.example.com
ENV ETCD http://etcd:2379
COPY --from=build /root/go/bin/casserole /usr/local/bin/casserole
CMD /go/bin/casserole server --address=0.0.0.0:80 --mirror-url=${UPSTREAM_SERVER} --peering-address=http://${HOSTNAME}:8080 --etcd=${ETCD} --passthrough='\.pom$,\.pom\.sha1$,\.xml$,\.xml\.sha1$'

