FROM golang:1.22 AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/agentd ./cmd/agentd

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/agentd /agentd
EXPOSE 8080
ENTRYPOINT ["/agentd"]
