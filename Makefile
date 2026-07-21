.PHONY: gen-proto

gen-proto:
	protoc --proto_path=proto \
		--go_out=. \
		--go_opt=module=github.com/swapnil404/orca \
		proto/orca.proto
