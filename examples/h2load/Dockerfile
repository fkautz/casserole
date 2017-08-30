#FROM fedora
#RUN dnf install gradle -y

FROM alpine
#RUN apk --update add gradle
RUN apk update \
    && apk add --no-cache openssl openssl-dev gcc g++ libgcc libstdc++ git jemalloc jemalloc-dev libev libev-dev autoconf automake make libtool \
    && git clone https://github.com/tatsuhiro-t/nghttp2.git \
    && cd nghttp2/ \
    && autoreconf -i && automake && autoconf && ./configure \
    && make && make install-strip \
    && cd .. && rm -rf nghttp2 \
    && apk del openssl-dev gcc g++ git jemalloc-dev libev-dev autoconf automake make libtool

ADD boom /bin/boom

ADD https://github.com/tianon/dockerfiles/raw/master/sleeping-beauty/sleeping-beauty /sleeping-beauty
RUN chmod 700 /sleeping-beauty
CMD /sleeping-beauty
