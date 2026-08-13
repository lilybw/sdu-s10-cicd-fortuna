FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum seed.sql ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server .

FROM alpine:3.20
RUN adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=build /app/server .
COPY --from=build /src/seed.sql .
USER appuser
EXPOSE 9000
ENTRYPOINT ["/app/server"]