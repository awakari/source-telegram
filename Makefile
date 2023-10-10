.PHONY: test clean
default: build

BINARY_FILE_NAME=producer-telegram
COVERAGE_FILE_NAME=cover.out
COVERAGE_TMP_FILE_NAME=cover.tmp

docker-tdlib:
	docker build -f tdlib.Dockerfile -t ghcr.io/awakari/tdlib:latest .
	docker push ghcr.io/awakari/tdlib:latest

vet:
	go vet

test: vet
	go test -race -cover -coverprofile=${COVERAGE_FILE_NAME} ./...
	cat ${COVERAGE_FILE_NAME} | grep -v _mock.go | grep -v logging.go | grep -v .pb.go > ${COVERAGE_FILE_NAME}.tmp
	mv -f ${COVERAGE_FILE_NAME}.tmp ${COVERAGE_FILE_NAME}
	go tool cover -func=${COVERAGE_FILE_NAME} | grep -Po '^total\:\h+\(statements\)\h+\K.+(?=\.\d+%)' > ${COVERAGE_TMP_FILE_NAME}
	./scripts/cover.sh
	rm -f ${COVERAGE_TMP_FILE_NAME}

build:
	CGO_ENABLED=0 GOOS=linux GOARCH= GOARM= go build -ldflags="-s -w" -o ${BINARY_FILE_NAME} main.go
	chmod ugo+x ${BINARY_FILE_NAME}

docker:
	docker build -t awakari/producer-telegram .

run:
	docker run \
		--rm \
		-it \
		--env-file env.txt \
		--name awakari-producer-telegram \
		--network host \
		awakari/producer-telegram

staging: docker
	./scripts/staging.sh

release: docker
	./scripts/release.sh

clean:
	go clean
	rm -f ${BINARY_FILE_NAME} ${COVERAGE_FILE_NAME}
