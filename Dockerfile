FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /specter ./cmd/specter

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /specter /specter
EXPOSE 8080
ENTRYPOINT ["/specter", "serve", "-demo", "-addr", "0.0.0.0:8080"]
