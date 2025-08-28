FROM ubuntu:22.04
RUN DEBIAN_FRONTEND=noninteractive apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install \
    ca-certificates libssl3 vim strace lsof curl jq git libpq-dev && \
    rm -rf /var/cache/apt /var/lib/apt/lists/*
ADD /tracker /app/tracker
ENV PATH "/app:$PATH"
ENTRYPOINT ["/app/tracker"]