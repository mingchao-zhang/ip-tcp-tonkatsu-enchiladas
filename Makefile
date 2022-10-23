all:
	export GO111MODULE="on"
	go build -o node ./cmd/node/main.go
clean:
	rm -fv node
	rm -fv *.txt
