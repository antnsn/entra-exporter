FROM golang:1.24-alpine AS build

WORKDIR /app

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o entra-exporter .

# Use Google's distroless as minimal base image
# https://github.com/GoogleContainerTools/distroless
FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=build /app/entra-exporter /entra-exporter

# Use non-root user for better security
USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/entra-exporter"]
