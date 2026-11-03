# learn-system-design starter: Go HTTP service compiled to a static binary on a distroless base.

# ---- build ------------------------------------------------------------------
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/server

# ---- runtime: no shell, no package manager, runs as uid 65532 ---------------
FROM gcr.io/distroless/static-debian12:nonroot AS runtime
COPY --from=build /out/app /app
ENV PORT=8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app"]
