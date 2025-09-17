# syntax=docker/dockerfile:1.6
FROM golang:1.24-alpine AS build
WORKDIR /src

# Кэшируем зависимости
COPY go.mod go.sum ./
RUN go mod download

# Сборка (если main.go в корне)
COPY . .
ARG VERSION=local
ARG COMMIT=dev
ARG DATE
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -a -trimpath -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${DATE}" \
    -o /out/app .

# Минимальный рантайм
FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=build /out/app /app/app
USER nonroot:nonroot
EXPOSE 8082
ENTRYPOINT ["/app/app"]