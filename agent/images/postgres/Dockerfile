ARG POSTGRES_VERSION=17
FROM postgres:${POSTGRES_VERSION}

RUN apt-get update \
    && apt-get install -y --no-install-recommends pgbackrest \
    && rm -rf /var/lib/apt/lists/*
