.PHONY: gen-proto postgres-image

gen-proto:
	protoc --proto_path=proto \
		--go_out=. \
		--go_opt=module=github.com/swapnil404/orca \
		proto/orca.proto

POSTGRES_VERSION ?= 17

postgres-image:
	docker build --build-arg POSTGRES_VERSION=$(POSTGRES_VERSION) \
		-t orca-postgres:$(POSTGRES_VERSION) agent/images/postgres
