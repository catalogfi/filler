build_daemons:
	mkdir -p ~/.cobi
	mkdir -p ~/.cobi/bin
	go build -tags netgo -ldflags '-s -w' -o ~/.cobi/bin ./cmd/daemon/executor ./cmd/daemon/strategy ./cmd/daemon/rpc
start: 
	~/.cobi/bin/rpc
build_cli:
	mkdir -p ~/.cobi
	mkdir -p ~/.cobi/bin
	go build -tags netgo -ldflags '-s -w' -o ~/.cobi/bin ./cmd/cli