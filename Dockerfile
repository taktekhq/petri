# Build the petri binary statically against musl so it runs unmodified inside
# the postgres:alpine image. CGO_ENABLED=0 means no glibc dependency.
# Deps are vendored so the build is fully offline — useful for restricted
# networks and for keeping the image reproducible without an extra fetch step.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -mod=vendor -o /out/petri ./cmd/petri

# Bundle petri with Postgres in a single image. Users replace
# `image: postgres:16` with `image: petri:postgres` in their compose file
# and get isolated forked databases per client connection — no other
# config required beyond the standard POSTGRES_USER/PASSWORD/DB env vars.
FROM postgres:16-alpine
COPY --from=build /out/petri /usr/local/bin/petri
COPY docker/petri-entrypoint.sh /usr/local/bin/petri-entrypoint.sh
RUN chmod +x /usr/local/bin/petri-entrypoint.sh

# Postgres runs on the loopback port; petri listens on the standard 5432
# and forwards to it. Only 5432 is exposed.
ENV PGPORT=5433
EXPOSE 5432

ENTRYPOINT ["/usr/local/bin/petri-entrypoint.sh"]
CMD ["postgres"]
