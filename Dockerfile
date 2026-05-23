# --- build stage ---
FROM golang:1.22-alpine AS builder
WORKDIR /src

# Кешируем зависимости
COPY go.mod ./
RUN go mod download

COPY . .

# go mod tidy создаст go.sum при первой сборке
RUN go mod tidy

# CGO выключен — статически линкуем в distroless
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" \
    -o /out/bot ./cmd/bot

# --- runtime stage ---
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=builder /out/bot /app/bot
USER nonroot:nonroot
ENTRYPOINT ["/app/bot"]
