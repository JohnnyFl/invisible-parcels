# build context: repo root (see docker-compose.yml)
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY contract contract
COPY backend backend
RUN CGO_ENABLED=0 go build -o /out/backend ./backend/cmd

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/backend /backend
ENTRYPOINT ["/backend"]
