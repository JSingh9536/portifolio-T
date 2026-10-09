FROM rust:1-slim AS build
WORKDIR /src
COPY . .
RUN cargo build --release --locked

FROM debian:bookworm-slim
COPY --from=build /src/target/release/uplift-sim /usr/local/bin/uplift-sim
USER nobody
ENTRYPOINT ["uplift-sim"]
