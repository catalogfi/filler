setupAndBuild:
	mkdir ~/.cobi
	mkdir ~/.cobi/bin
	go build -tags netgo -ldflags '-s -w' -o ~/.cobi/bin ./cmd/daemon/executor ./cmd/daemon/strategy ./cmd/daemon/rpc
start: 
	~/.cobi/bin/rpc
build:
	go build -tags netgo -ldflags '-s -w' -o ~/.cobi/bin ./cmd/daemon/executor ./cmd/daemon/strategy ./cmd/daemon/rpc