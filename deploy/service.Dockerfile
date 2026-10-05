# Build the static executable from the fixed source commit on Fedora.
FROM scratch
COPY --chmod=755 ani-modeldev-service /ani-modeldev-service
USER 10001:10001
ENTRYPOINT ["/ani-modeldev-service"]
