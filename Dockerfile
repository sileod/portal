FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /portal ./cmd/portal

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=build /portal /usr/local/bin/portal
EXPOSE 8080
ENTRYPOINT ["portal","hub"]
