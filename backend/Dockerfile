FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/triage ./cmd/server

FROM alpine:3.21
WORKDIR /app
COPY --from=build /out/triage /app/triage
EXPOSE 8080
CMD ["/app/triage"]
