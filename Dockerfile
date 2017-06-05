FROM alpine:3.5

RUN apk --no-cache add ca-certificates
COPY ./kube-traefik /kube-traefik

ENTRYPOINT ["/kube-traefik"]
