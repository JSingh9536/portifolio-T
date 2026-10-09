# Build on the same Debian release as the runtime image so glibc versions match.
FROM public.ecr.aws/docker/library/rust:1-slim-bookworm AS build
WORKDIR /src
COPY . .
RUN cargo build --release --locked

FROM public.ecr.aws/docker/library/debian:bookworm-slim
COPY --from=build /src/target/release/uplift-sim /usr/local/bin/uplift-sim
USER nobody
ENTRYPOINT ["uplift-sim"]
