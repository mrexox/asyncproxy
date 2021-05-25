VERSION := 1.0
NAME    := asyncproxy

build:
	go build -ldflags '-w -s' -o "$(NAME)"

docker-build:
	docker build --tag "$(NAME):$(VERSION)" .

test:
	go test
