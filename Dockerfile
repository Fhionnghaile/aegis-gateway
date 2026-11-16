# Multi-stage -> FROM scratch: a few-MB, non-root, shell-less image.
FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /gateway ./cmd/gateway

FROM scratch
COPY --from=build /gateway /gateway
COPY schemas /schemas
USER 65534:65534
EXPOSE 8080
ENTRYPOINT ["/gateway"]
