FROM golang
WORKDIR /tmp/loggregator
ADD . /tmp/loggregator
RUN cd /tmp/loggregator && \
    go build -o /bin/loggregator && \
		rm -rf /tmp/loggregator
ENTRYPOINT ["/bin/loggregator"]
