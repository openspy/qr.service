FROM golang:latest as build
WORKDIR /app
COPY src/ src
WORKDIR /app/src
RUN go mod download
RUN go build -o ../qr-service

FROM golang:latest
WORKDIR /app
COPY --from=build /app/qr-service qr-service
COPY run.sh .
ENTRYPOINT ["/app/qr-service"]