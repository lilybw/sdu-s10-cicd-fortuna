FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server .

FROM alpine:3.20
RUN adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=build /app/server .
COPY --from=build /src/static ./static
COPY --from=build /src/templates ./templates
USER appuser
EXPOSE 8080
ENTRYPOINT ["/app/server"]