BIN := isg_exporter
build:
	GOOS=linux go build -o build/$(BIN).linux
	GOOS=darwin go build -o build/$(BIN).darwin
