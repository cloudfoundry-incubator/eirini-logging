FROM opensuse:leap
ADD binaries/kubectl /bin/kubectl
RUN chmod +x /bin/kubectl
ADD binaries/eirini-logging /bin/eirini-logging
ENTRYPOINT ["/bin/eirini-logging"]
