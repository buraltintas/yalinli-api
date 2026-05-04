FROM golang:1.25.7-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /bin/api ./cmd/api

FROM alpine:3.20
RUN adduser -D appuser
USER appuser
WORKDIR /app
COPY --from=build /bin/api /app/api
EXPOSE 8080
CMD ["/app/api"]
