# Public demo image: samples preloaded, visitors can check text and verify receipts but not write.
# For a private instance that can add sources, drop "-readonly" below.
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /revenant ./cmd/revenant && mkdir /seed

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /revenant /revenant
COPY --from=build --chown=nonroot:nonroot /seed /data
# With $PORT set the server binds all interfaces; hosting platforms override PORT.
ENV REVENANT_DATA=/data PORT=8080
EXPOSE 8080
ENTRYPOINT ["/revenant", "serve", "-demo", "-readonly"]
