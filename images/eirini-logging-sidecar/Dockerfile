FROM busybox

RUN wget https://storage.googleapis.com/kubernetes-release/release/v1.15.0/bin/linux/amd64/kubectl -O /bin/kubectl && \
    chmod +x /bin/kubectl

ENTRYPOINT [ "/bin/kubectl" ]
