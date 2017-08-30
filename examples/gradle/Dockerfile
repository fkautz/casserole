FROM fedora
RUN dnf install gradle -y

ADD . /app
ADD https://github.com/tianon/dockerfiles/raw/master/sleeping-beauty/sleeping-beauty /sleeping-beauty
ENV TERM xterm
WORKDIR /app
RUN chmod 700 /sleeping-beauty
CMD /sleeping-beauty
