FROM golang:1.24.4 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/agentd ./cmd/agentd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/agentd /agentd
EXPOSE 8080
ENTRYPOINT ["/agentd"]

