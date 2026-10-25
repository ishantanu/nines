# Official Debian image; the fixture installs Debian-maintained OpenLDAP packages.
FROM debian:trixie-slim@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad
RUN printf 'slapd slapd/no_configuration boolean true\n' | debconf-set-selections \
    && apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends slapd ldap-utils ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY bootstrap.sh /bootstrap.sh
ENTRYPOINT ["/bin/sh", "/bootstrap.sh"]
