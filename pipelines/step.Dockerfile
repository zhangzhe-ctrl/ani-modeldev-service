# The static executable is built from the fixed source commit on Fedora.
FROM scratch
COPY ani-modeldev-step /ani-modeldev-step
USER 10001:10001
ENTRYPOINT ["/ani-modeldev-step"]
