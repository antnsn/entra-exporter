VERSION := 0.1.0
LDFLAGS := -X main.Version=$(VERSION)

.PHONY: all build docker clean test

all: build

build:
	go build -ldflags "$(LDFLAGS)" -o entra-exporter

docker:
	docker build -t entra-exporter:$(VERSION) .

clean:
	rm -f entra-exporter

test:
	go test -v ./...
