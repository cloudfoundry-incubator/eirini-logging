FROM opensuse:leap
ADD binaries/eirini-logging /bin/eirini-logging
ENTRYPOINT ["/bin/eirini-logging"]
