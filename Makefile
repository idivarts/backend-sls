.PHONY: build clean deploy gomodgen

build: gomodgen
	go mod tidy
	export GO111MODULE=on
	# GOARCH=arm64 GOOS=linux CGO_ENABLED=0 go build -tags lambda.norpc -o bin/hello/bootstrap hello/main.go
	# GOARCH=arm64 GOOS=linux CGO_ENABLED=0 go build -tags lambda.norpc -o bin/world/bootstrap world/main.go

	# env GOARCH=amd64 GOOS=linux go build -ldflags="-s -w" -o bin/hello hello/main.go
	# env GOARCH=amd64 GOOS=linux go build -ldflags="-s -w" -o bin/world world/main.go

clean:
	rm -rf ./bin ./vendor go.sum

deploy: clean build
	sls deploy --verbose

gomodgen:
	chmod u+x gomod.sh
	./gomod.sh
