# Build the petri binary statically against musl so it runs unmodified inside
# the postgres:alpine image. CGO_ENABLED=0 means no glibc dependency.
#
# `go build` auto-uses ./vendor/ if it's present in the build context (Go
# >=1.14 behaviour). That's a handy escape hatch for restricted networks —
# pre-run `go mod vendor` and the build is fully offline — but the common
# path fetches modules from the configured proxy on demand. No env var or
# flag toggle either way.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/petri ./cmd/petri

# Bundle petri with Postgres in a single image. Users replace
# `image: postgres:16.4-alpine` with `image: petri:postgres` in compose, or
# `FROM postgres:16.4-alpine` with `FROM petri:postgres` in their own
# Dockerfile. Connections to 5432 behave like regular Postgres; connections
# to 5433 land on a freshly-forked database per connection — same image,
# different port, no app-side config required beyond the standard
# POSTGRES_USER/PASSWORD/DB env vars. Pinned to 16.4 because that's the
# only Postgres version supported.
FROM postgres:16.4-alpine
COPY --from=build /out/petri /usr/local/bin/petri
COPY docker/petri-entrypoint.sh /usr/local/bin/petri-entrypoint.sh
RUN chmod +x /usr/local/bin/petri-entrypoint.sh

# Postgres listens on the loopback port; petri fronts it with two TCP
# listeners — 5432 passthrough (drop-in) and 5433 fork-per-connection.
# Postgres is pushed off 5433 onto 5434 so 5433 is free for petri.
ENV PGPORT=5434
EXPOSE 5432 5433

ENTRYPOINT ["/usr/local/bin/petri-entrypoint.sh"]
CMD ["postgres"]
