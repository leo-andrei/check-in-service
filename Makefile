.PHONY: run build test docker-up docker-down setup-rabbitmq

run:
	go run cmd/api/main.go

build:
	go build -o bin/checkin-service cmd/api/main.go

test:
	go test -v ./...

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f checkin-service

setup-rabbitmq:
	# Wait for RabbitMQ to be ready
	@echo "Waiting for RabbitMQ..."
	@until curl -s http://localhost:15672 > /dev/null; do sleep 1; done
	@echo "RabbitMQ is ready!"

clean:
	docker compose down -v
	rm -rf bin/
