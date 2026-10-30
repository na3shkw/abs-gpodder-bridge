FROM golang:1.25-trixie AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bridge ./cmd/bridge

FROM gcr.io/distroless/static-debian13:nonroot
LABEL org.opencontainers.image.source=https://github.com/na3shkw/abs-gpodder-bridge
COPY --from=build /out/bridge /usr/local/bin/bridge
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/bridge"]
