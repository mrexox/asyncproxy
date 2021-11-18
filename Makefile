# Don't forget to change this var when tagging a commit
VERSION       := 2.9.1
IMAGE_NAME    ?= asyncproxy

build:
	go build -ldflags '-w -s' -o "$(IMAGE_NAME)"

test:
	go test ./...

docker-build:
	docker build --tag "$(IMAGE_NAME):builder" \
							 --target builder \
							 --cache-from "$(IMAGE_NAME):builder" \
							 .
	docker build --tag "$(IMAGE_NAME):$(VERSION)" \
							 --tag "$(IMAGE_NAME):latest" \
							 --cache-from "$(IMAGE_NAME):builder" \
							 .

docker-push:
	docker push "$(IMAGE_NAME):$(VERSION)"
	docker push "$(IMAGE_NAME):latest"

dev-db-up:
	docker run --rm -d \
		--name asyncproxy-postgresql \
		-p 5432:5432 \
		-e POSTGRES_USER=postgres \
		-e POSTGRES_PASSWORD=postgres \
		-e POSTGRES_DB=asyncproxy \
		postgres:13

dev-db-migrate:
	goose -dir migrations postgres \
	'host=localhost port=5432 user=postgres password=postgres dbname=asyncproxy sslmode=disable' \
	up
