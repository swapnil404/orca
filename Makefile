.PHONY: gen-proto

gen-proto:
	protoc --proto_path=proto \
		--go_out=. \
		--go_opt=module=github.com/betterorca/betterorca \
		proto/orca.proto
