VERSION       := 1.0
IMAGE_NAME    ?= asyncproxy

build:
	go build -ldflags '-w -s' -o "$(NAME)"

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

test:
	go test github.com/evilmartians/asyncproxy/proxy
