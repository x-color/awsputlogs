.PHONY: format
format:
	go fmt

.PHONY: lint
lint: format
	go vet
	staticcheck

.PHONY: test
test: lint
	docker-compose up -d
	@echo Waiting for the localstack to start running...
	sleep 10
	@echo ----------start testing------------
	-AWS_ACCESS_KEY_ID=DUMMY AWS_SECRET_ACCESS_KEY=DUMMY go test
	@echo ----------finished testing------------
	docker-compose stop

.PHONY: build
build: format
	go build -o awsputlogs main.go

.PHONY: install
install: format
	go install
