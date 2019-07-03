FROM golang
WORKDIR /tmp/eirini-logging
ADD . /tmp/eirini-logging
RUN cd /tmp/eirini-logging && \
    go build -o /bin/eirini-logging && \
		rm -rf /tmp/eirini-logging
ENTRYPOINT ["/bin/eirini-logging"]
