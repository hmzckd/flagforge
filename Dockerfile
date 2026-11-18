FROM golang:1.27.1-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/flagforge ./cmd/flagforge

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/flagforge /flagforge
EXPOSE 8080
ENTRYPOINT ["/flagforge"]
