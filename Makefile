all:
	export GO111MODULE="on"
	go build -o node ./cmd
clean:
	rm -fv node
	rm -fv *.txt
