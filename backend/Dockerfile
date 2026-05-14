FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /announcement-api ./cmd/api

FROM alpine:3.20
WORKDIR /app
COPY --from=build /announcement-api /announcement-api
EXPOSE 8080
CMD ["/announcement-api"]
