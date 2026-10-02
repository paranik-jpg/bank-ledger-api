# syntax=docker/dockerfile:1

# Building the app from source
FROM golang:1.27-alpine AS build-stage

WORKDIR /app

ENV GOTOOLCHAIN=auto

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/api ./cmd/api/main.go


# Copying the compiled binary
FROM alpine:latest AS final-stage

WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata

COPY --from=build-stage /app/api /app/api

COPY --from=build-stage /app/db/migration /app/db/migration

EXPOSE 8080

CMD ["/app/api"]
